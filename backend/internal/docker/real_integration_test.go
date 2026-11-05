package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"deployfox/internal/platform"
	"deployfox/internal/security"
)

// This test executes real Docker builds, containers, Caddy route changes and PostgreSQL restore.
// It requires a disposable agent container, /work, /routes, /backups, Docker socket and deployfox-apps.
func TestRealDockerLifecycle(t *testing.T) {
	if os.Getenv("RUN_DOCKER_TESTS") != "1" {
		t.Skip("explicit RUN_DOCKER_TESTS=1 required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	a := Agent{}
	builder := os.Getenv("DOCKER_TEST_BUILDER")
	if builder == "" {
		builder = "deployfox"
	}
	pid := security.ID()
	domain := "qa-" + pid + ".deployfox.test"
	p := platform.Project{ID: pid, Port: 8080, HealthPath: "/", MemoryMB: 64, CPU: 0.25, Domain: domain}
	defer a.deleteProject(context.Background(), Request{Project: p})
	if _, err := command(ctx, "", nil, "docker", "buildx", "inspect", builder); err != nil {
		if out, e := command(ctx, "", nil, "docker", "buildx", "create", "--name", "deployfox", "--driver", "docker-container", "--driver-opt", "memory=256m,cpu-period=100000,cpu-quota=100000"); e != nil {
			t.Fatal(out, e)
		}
	}
	build := func(version string) string {
		t.Helper()
		dir, err := os.MkdirTemp("/work", "qa-source-")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		dockerfile := fmt.Sprintf("FROM alpine:3.22\nRUN apk add --no-cache busybox-extras\nRUN mkdir /www && printf '%s' > /www/index.html\nCMD [\"/bin/busybox-extras\",\"httpd\",\"-f\",\"-p\",\"8080\",\"-h\",\"/www\"]\n", version)
		if version == "background" {
			dockerfile = "FROM alpine:3.22\nCMD [\"sleep\",\"3600\"]\n"
		}
		if err = os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0600); err != nil {
			t.Fatal(err)
		}
		image := "deployfox/" + pid + ":" + security.ID()
		if out, e := command(ctx, "", nil, "docker", "buildx", "build", "--builder", builder, "--load", "-t", image, dir); e != nil {
			t.Fatal("real Docker build", out, e)
		}
		t.Cleanup(func() { command(context.Background(), "", nil, "docker", "image", "rm", image) })
		return image
	}
	checkRoute := func(want string) {
		t.Helper()
		req, _ := http.NewRequestWithContext(ctx, "GET", "http://deployfox-caddy:80", nil)
		req.Host = domain
		resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 || string(body) != want {
			t.Fatalf("route got HTTP %d %q, want %q", resp.StatusCode, body, want)
		}
	}
	image1 := build("version-one")
	first, err := a.deploy(ctx, Request{Project: p, DeploymentID: security.ID(), Image: image1})
	if err != nil {
		t.Fatal("first deployment", err)
	}
	p.ActiveContainer = first.Container
	p.ActiveImage = image1
	configJSON, err := command(ctx, "", nil, "docker", "inspect", "--format", "{{json .HostConfig}}", first.Container)
	if err != nil {
		t.Fatal(err)
	}
	var limits struct {
		Privileged bool
		Memory     int64
		NanoCpus   int64
		PidsLimit  int64
		Binds      []string
	}
	if err = json.Unmarshal([]byte(configJSON), &limits); err != nil || limits.Privileged || limits.Memory != 64*1024*1024 || limits.NanoCpus != 250000000 || limits.PidsLimit != 256 || len(limits.Binds) != 0 {
		t.Fatal("container isolation/resource limits not applied", err)
	}
	host, err := hostMetrics(ctx)
	if err != nil {
		t.Fatal("host metrics", err)
	}
	var hostData struct {
		Available bool    `json:"available"`
		CPU       float64 `json:"cpu_percent"`
		Memory    uint64  `json:"memory_total"`
	}
	if json.Unmarshal([]byte(host.Output), &hostData) != nil || !hostData.Available || hostData.CPU < 0 || hostData.CPU > 100 || hostData.Memory == 0 {
		t.Fatal("invalid real host metrics")
	}
	checkRoute("version-one")
	image2 := build("version-two")
	previousDomain := domain
	domain = "release-" + pid + ".deployfox.test"
	p.Domain = domain
	second, err := a.deploy(ctx, Request{Project: p, DeploymentID: security.ID(), Image: image2})
	if err != nil {
		t.Fatal("second deployment", err)
	}
	p.ActiveContainer = second.Container
	p.ActiveImage = image2
	checkRoute("version-two")
	oldRequest, _ := http.NewRequestWithContext(ctx, "GET", "http://deployfox-caddy:80", nil)
	oldRequest.Host = previousDomain
	oldResponse, err := (&http.Client{Timeout: 5 * time.Second}).Do(oldRequest)
	if err != nil {
		t.Fatal(err)
	}
	oldBody, _ := io.ReadAll(oldResponse.Body)
	oldResponse.Body.Close()
	if strings.Contains(string(oldBody), "version-") {
		t.Fatal("previous domain route survived successful switch")
	}
	badRoute := filepath.Join("/routes", pid+"-invalid-test.caddy")
	if err = os.WriteFile(badRoute, []byte("invalid.test {\n this_directive_does_not_exist\n}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(badRoute)
	if _, err = a.deploy(ctx, Request{Project: p, DeploymentID: security.ID(), Image: image1}); err == nil {
		t.Fatal("invalid Caddy configuration was accepted")
	}
	checkRoute("version-two")
	if err = os.Remove(badRoute); err != nil {
		t.Fatal(err)
	}
	broken := p
	broken.Port = 65000
	failureCtx, stop := context.WithTimeout(ctx, 4*time.Second)
	_, err = a.deploy(failureCtx, Request{Project: broken, DeploymentID: security.ID(), Image: image2})
	stop()
	if err == nil {
		t.Fatal("bad healthcheck accepted")
	}
	checkRoute("version-two")
	rolled, err := a.deploy(ctx, Request{Project: p, DeploymentID: security.ID(), Image: image1})
	if err != nil {
		t.Fatal("rollback", err)
	}
	p.ActiveContainer = rolled.Container
	checkRoute("version-one")
	stats, err := a.operate(ctx, Request{Action: "metrics", Project: p})
	if err != nil || !strings.Contains(stats.Output, "available") {
		t.Fatal("real container metrics", err)
	}
	p.ServiceType = "worker"
	p.Domain = ""
	backgroundImage := build("background")
	background, err := a.deploy(ctx, Request{Project: p, DeploymentID: security.ID(), Image: backgroundImage})
	if err != nil {
		t.Fatal("background deployment", err)
	}
	if out, err := command(ctx, "", nil, "docker", "inspect", "--format", "{{.State.Running}}", background.Container); err != nil || strings.TrimSpace(out) != "true" {
		t.Fatal("background process not running", out, err)
	}
	if _, err = os.Stat(filepath.Join("/routes", pid+".caddy")); !os.IsNotExist(err) {
		t.Fatal("public route was not removed for background application")
	}
	if _, err = a.deleteProject(ctx, Request{Project: p}); err != nil {
		t.Fatal("project deletion", err)
	}
}

func TestRealManagedPostgresRestore(t *testing.T) {
	if os.Getenv("RUN_DOCKER_TESTS") != "1" {
		t.Skip("explicit RUN_DOCKER_TESTS=1 required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	id := security.ID()
	name := "dfdb-" + id
	r := Request{Action: "database-create", Project: platform.Project{ID: id}, Database: DatabaseSpec{Engine: "postgres", Version: "16", Password: security.Token()}}
	a := Agent{}
	defer func() {
		command(context.Background(), "", nil, "docker", "rm", "-f", name)
		command(context.Background(), "", nil, "docker", "volume", "rm", "dfdb-"+id+"-data")
	}()
	if _, err := a.database(ctx, r); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"database-stop", "database-start", "database-restart"} {
		operation := r
		operation.Action = action
		if _, err := a.database(ctx, operation); err != nil {
			t.Fatalf("database lifecycle %s: %v", action, err)
		}
	}
	ready := false
	for i := 0; i < 30; i++ {
		if _, err := command(ctx, "", nil, "docker", "exec", name, "pg_isready", "-h", "127.0.0.1", "-U", "app", "-d", "app"); err == nil {
			ready = true
			break
		}
		time.Sleep(time.Second)
	}
	if !ready {
		t.Fatal("PostgreSQL not ready")
	}
	if out, err := command(ctx, "", nil, "docker", "exec", name, "psql", "-U", "app", "-d", "app", "-c", "CREATE TABLE recovery_probe(value text); INSERT INTO recovery_probe VALUES ('survived-backup');"); err != nil {
		t.Fatal(out, err)
	}
	r.Action = "database-backup"
	result, err := a.database(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	filePath := strings.TrimSuffix(result.Output, " (restore verification required)")
	defer os.Remove(filePath)
	defer os.Remove(filePath + ".sha256")
	if _, err = command(ctx, "", nil, "docker", "exec", name, "createdb", "-U", "app", "restored"); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(filePath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", name, "pg_restore", "-U", "app", "-d", "restored", "--exit-on-error")
	cmd.Stdin = file
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("actual pg_restore: %s %v", out, err)
	}
	out, err := command(ctx, "", nil, "docker", "exec", name, "psql", "-U", "app", "-d", "restored", "-Atc", "SELECT value FROM recovery_probe")
	if err != nil || strings.TrimSpace(out) != "survived-backup" {
		t.Fatal("restore data verification failed", out, err)
	}
	if _, err = command(ctx, "", nil, "docker", "exec", name, "psql", "-U", "app", "-d", "app", "-c", "DELETE FROM recovery_probe"); err != nil {
		t.Fatal(err)
	}
	r.Action = "database-restore"
	r.Backup = filepath.Base(filePath)
	if _, err = a.database(ctx, r); err != nil {
		t.Fatal("agent restore failed", err)
	}
	out, err = command(ctx, "", nil, "docker", "exec", name, "psql", "-U", "app", "-d", "app", "-Atc", "SELECT value FROM recovery_probe")
	if err != nil || strings.TrimSpace(out) != "survived-backup" {
		t.Fatal("agent restore data verification", out, err)
	}
}
