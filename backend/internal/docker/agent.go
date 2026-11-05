package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"deployfox/internal/github"
	"deployfox/internal/security"
)

type Agent struct {
	Token   string
	mu      sync.Mutex
	buildMu sync.Mutex
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func command(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	// Build planning and child tools must never inherit control-plane credentials.
	for _, key := range []string{"PATH", "HOME", "DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_CONFIG", "SSL_CERT_FILE", "SSL_CERT_DIR", "LANG"} {
		if value, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	cmd.Env = append(cmd.Env, env...)
	var out boundedBuffer
	var writer io.Writer = &out
	var lines *buildLineWriter
	if emit, ok := ctx.Value(buildOutputKey{}).(func(string)); ok {
		lines = &buildLineWriter{emit: emit}
		writer = io.MultiWriter(&out, lines)
	}
	cmd.Stdout = writer
	cmd.Stderr = writer
	err := cmd.Run()
	if lines != nil {
		lines.flush()
	}
	return out.String(), err
}

type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len() < 1<<20 {
		available := (1 << 20) - b.Len()
		if len(p) > available {
			p = p[:available]
		}
		b.Buffer.Write(p)
	}
	return n, nil
}

func (a *Agent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !security.Equal(r.Header.Get("Authorization"), "Bearer "+a.Token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.URL.Path == "/health" && r.Method == "GET" {
		w.Write([]byte("ok"))
		return
	}
	if r.Method != "POST" || r.URL.Path != "/operations" {
		http.NotFound(w, r)
		return
	}
	var req Request
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req) != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.Action != "host-metrics" && req.Action != "caddy-check" && !uuidPattern.MatchString(req.Project.ID) {
		http.Error(w, "invalid project", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	if req.Action == "build" {
		a.buildMu.Lock()
		defer a.buildMu.Unlock()
	} else if req.Action != "host-metrics" && req.Action != "caddy-check" && req.Action != "metrics" && req.Action != "logs" {
		a.mu.Lock()
		defer a.mu.Unlock()
	}
	if req.Action == "build" && r.Header.Get("Accept") == "application/x-ndjson" {
		a.streamBuild(w, r, ctx, req)
		return
	}
	result, err := a.operate(ctx, req)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		result.Error = err.Error()
		w.WriteHeader(http.StatusBadGateway)
	}
	json.NewEncoder(w).Encode(result)
}
func (a *Agent) operate(ctx context.Context, r Request) (Result, error) {
	if strings.HasPrefix(r.Action, "database-") {
		return a.database(ctx, r)
	}
	switch r.Action {
	case "host-metrics":
		return hostMetrics(ctx)
	case "caddy-check":
		if _, err := command(ctx, "", nil, "docker", "exec", "deployfox-caddy", "caddy", "validate", "--config", "/etc/caddy/Caddyfile"); err != nil {
			return Result{}, errors.New("caddy configuration validation failed; inspect Caddy logs")
		}
		return Result{Output: `{"available":true,"valid":true}`}, nil
	case "delete":
		return a.deleteProject(ctx, r)
	case "metrics":
		if err := ownedContainer(ctx, r.Project.ActiveContainer, r.Project.ID); err != nil {
			return Result{}, err
		}
		stats, err := command(ctx, "", nil, "docker", "stats", "--no-stream", "--format", "{{json .}}", r.Project.ActiveContainer)
		if err != nil {
			return Result{}, err
		}
		state, err := command(ctx, "", nil, "docker", "inspect", "--format", `{"status":{{json .State.Status}},"started_at":{{json .State.StartedAt}},"restarts":{{.RestartCount}},"health":{{if .State.Health}}{{json .State.Health.Status}}{{else}}"none"{{end}}}`, r.Project.ActiveContainer)
		if err != nil {
			return Result{}, err
		}
		return Result{Output: `{"available":true,"stats":` + strings.TrimSpace(stats) + `,"state":` + strings.TrimSpace(state) + `}`}, nil
	case "build":
		return a.build(ctx, r)
	case "deploy":
		return a.deploy(ctx, r)
	case "start", "stop", "restart":
		if err := ownedContainer(ctx, r.Project.ActiveContainer, r.Project.ID); err != nil {
			return Result{}, err
		}
		_, err := command(ctx, "", nil, "docker", r.Action, r.Project.ActiveContainer)
		return Result{}, err
	case "logs":
		if err := ownedContainer(ctx, r.Project.ActiveContainer, r.Project.ID); err != nil {
			return Result{}, err
		}
		out, err := command(ctx, "", nil, "docker", "logs", "--tail", "200", "--timestamps", r.Project.ActiveContainer)
		return Result{Output: out}, err
	default:
		return Result{}, errors.New("unsupported operation")
	}
}
func ownedContainer(ctx context.Context, name, pid string) error {
	if !strings.HasPrefix(name, "df-"+pid+"-") {
		return errors.New("invalid container name")
	}
	out, err := command(ctx, "", nil, "docker", "inspect", "--format", `{{index .Config.Labels "deployfox.project"}}`, name)
	if err != nil || strings.TrimSpace(out) != pid {
		return errors.New("container ownership mismatch")
	}
	return nil
}
func (a *Agent) build(ctx context.Context, r Request) (Result, error) {
	if !uuidPattern.MatchString(r.DeploymentID) {
		return Result{}, errors.New("invalid deployment id")
	}
	image := "deployfox/" + r.Project.ID + ":" + r.DeploymentID
	if r.Project.SourceType == "image" {
		if !security.ImagePattern.MatchString(r.Project.Source) || strings.HasPrefix(r.Project.Source, "-") {
			return Result{}, errors.New("invalid image")
		}
		out, err := command(buildOutputContext(ctx), "", nil, "docker", "pull", r.Project.Source)
		if err != nil {
			return Result{Output: out, Retryable: transientBuildOutput(out)}, errors.New("image pull failed")
		}
		_, err = command(ctx, "", nil, "docker", "tag", r.Project.Source, image)
		return Result{Image: image, Output: out}, err
	}
	if err := security.Repository(r.Project.Source); err != nil {
		return Result{}, err
	}
	if _, err := command(ctx, "", nil, "docker", "buildx", "inspect", "deployfox"); err != nil {
		if _, err = command(ctx, "", nil, "docker", "buildx", "create", "--name", "deployfox", "--driver", "docker-container", "--driver-opt", "memory=2g,cpu-period=100000,cpu-quota=100000"); err != nil {
			return Result{}, errors.New("cannot initialize resource-limited BuildKit builder")
		}
	}
	if !security.RelativePath(r.Project.BuildContext) || !security.RelativePath(r.Project.Dockerfile) {
		return Result{}, errors.New("invalid build path")
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", "github.com")
	if err != nil {
		return Result{}, err
	}
	for _, ip := range ips {
		if !security.PublicIP(ip) {
			return Result{}, errors.New("repository DNS points to a private address")
		}
	}
	dir, err := os.MkdirTemp("/work", "build-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(dir)
	// Git hooks, submodules, local protocols, redirects and interactive credential prompts are disabled.
	env := []string{"GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}
	token := r.GitToken
	if token == "" && os.Getenv("GITHUB_APP_ID") != "" {
		var e error
		token, e = github.InstallationToken(ctx)
		if e != nil {
			return Result{}, e
		}
	}
	if token != "" {
		askpass := filepath.Join(dir, "askpass.sh")
		if err = os.WriteFile(askpass, []byte("#!/bin/sh\ncase \"$1\" in *Username*) printf '%s' x-access-token;; *) printf '%s' \"$DF_GIT_TOKEN\";; esac\n"), 0700); err != nil {
			return Result{}, err
		}
		env = append(env, "GIT_ASKPASS="+askpass, "DF_GIT_TOKEN="+token)
	}
	repo := filepath.Join(dir, "repo")
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_./-]{0,127}$`).MatchString(r.Project.Branch) || strings.Contains(r.Project.Branch, "..") {
		return Result{}, errors.New("invalid branch")
	}
	cloneOutput, cloneErr := command(ctx, "", env, "git", "-c", "protocol.file.allow=never", "-c", "http.followRedirects=false", "clone", "--depth", "1", "--single-branch", "--branch", r.Project.Branch, "--", r.Project.Source, repo)
	err = cloneErr
	if err != nil {
		if token != "" {
			cloneOutput = strings.ReplaceAll(cloneOutput, token, "[REDACTED]")
		}
		return Result{Output: cloneOutput, Retryable: transientBuildOutput(cloneOutput)}, errors.New("repository clone failed")
	}
	if r.CommitSHA != "" {
		if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(r.CommitSHA) {
			return Result{}, errors.New("invalid commit")
		}
		if _, err = command(ctx, repo, env, "git", "-c", "http.followRedirects=false", "fetch", "--depth", "1", "origin", r.CommitSHA); err != nil {
			return Result{}, errors.New("cannot fetch webhook commit")
		}
		if _, err = command(ctx, repo, nil, "git", "checkout", "--detach", r.CommitSHA); err != nil {
			return Result{}, errors.New("cannot checkout webhook commit")
		}
	}
	commit, err := command(ctx, repo, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return Result{}, err
	}
	buildCtx, err := containedPath(repo, r.Project.BuildContext)
	if err != nil {
		return Result{}, err
	}
	dockerfile, err := containedPath(repo, r.Project.Dockerfile)
	buildArgs := []string{"buildx", "build", "--builder", "deployfox", "--load", "--progress", "plain", "--tag", image}
	if r.Project.BuildMethod == "buildpack" || os.IsNotExist(err) {
		dockerfile = filepath.Join(dir, "railpack-plan.json")
		_, err = command(buildOutputContext(ctx), repo, []string{"HOME=" + dir, "MISE_DATA_DIR=" + filepath.Join(dir, "mise")}, "railpack", "prepare", buildCtx, "--plan-out", dockerfile, "--info-out", filepath.Join(dir, "railpack-info.json"))
		if err != nil {
			return Result{}, errors.New("railpack could not detect or prepare this project; add a Dockerfile or an explicit start command")
		}
		buildArgs = append(buildArgs, "--build-arg", "BUILDKIT_SYNTAX=ghcr.io/railwayapp/railpack-frontend:v0.43.0", "--build-arg", "cache-key="+r.Project.ID)
	} else if err != nil {
		return Result{}, err
	}
	buildArgs = append(buildArgs, "--file", dockerfile, buildCtx)
	out, err := command(buildOutputContext(ctx), repo, nil, "docker", buildArgs...)
	if err != nil {
		return Result{Output: out, Retryable: transientBuildOutput(out)}, errors.New("docker build failed; see build output")
	}
	return Result{Image: image, Commit: strings.TrimSpace(commit), Output: out}, nil
}
func containedPath(root, rel string) (string, error) {
	p, err := filepath.EvalSymlinks(filepath.Join(root, rel))
	if err != nil {
		return "", err
	}
	back, err := filepath.Rel(root, p)
	if err != nil || back == ".." || strings.HasPrefix(back, ".."+string(filepath.Separator)) {
		return "", errors.New("build path escapes repository")
	}
	return p, nil
}
func (a *Agent) deploy(ctx context.Context, r Request) (Result, error) {
	p := r.Project
	if !uuidPattern.MatchString(r.DeploymentID) || !strings.HasPrefix(r.Image, "deployfox/"+p.ID+":") || !security.ImagePattern.MatchString(r.Image) {
		return Result{}, errors.New("image does not belong to project")
	}
	if p.MemoryMB < 64 || p.MemoryMB > 32768 || p.CPU <= 0 || p.CPU > 32 || p.Port < 1 || p.Port > 65535 {
		return Result{}, errors.New("invalid resources")
	}
	if p.Domain != "" && (!security.Domain(p.Domain) || p.Domain == os.Getenv("PANEL_DOMAIN")) {
		return Result{}, errors.New("invalid or reserved domain")
	}
	name := "df-" + p.ID + "-" + strings.ReplaceAll(r.DeploymentID, "-", "")[:22]
	if p.ServiceType == "worker" && p.Domain != "" {
		return Result{}, errors.New("background process cannot have a domain")
	}
	backgroundStopped := false
	if p.ServiceType == "worker" && p.ActiveContainer != "" {
		if err := ownedContainer(ctx, p.ActiveContainer, p.ID); err != nil {
			return Result{}, err
		}
		if _, err := command(ctx, "", nil, "docker", "stop", "--time", "20", p.ActiveContainer); err != nil {
			return Result{}, err
		}
		backgroundStopped = true
	}
	backgroundCommitted := false
	defer func() {
		if backgroundStopped && !backgroundCommitted {
			restore, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			command(restore, "", nil, "docker", "start", p.ActiveContainer)
		}
	}()
	args := []string{"create", "--name", name, "--label", "deployfox.project=" + p.ID, "--label", "deployfox.deployment=" + r.DeploymentID, "--network", "deployfox-apps", "--memory", fmt.Sprintf("%dm", p.MemoryMB), "--cpus", fmt.Sprint(p.CPU), "--pids-limit", "256", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true", "--restart", "unless-stopped", "--log-opt", "max-size=10m", "--log-opt", "max-file=3"}
	// Env values travel through stdin to docker, never as process arguments.
	var input strings.Builder
	for key, value := range r.Environment {
		if !security.EnvPattern.MatchString(key) || strings.ContainsAny(value, "\n\r\x00") {
			return Result{}, errors.New("invalid environment value")
		}
		fmt.Fprintf(&input, "%s=%s\n", key, value)
	}
	file, err := os.CreateTemp("/work", "env-")
	if err != nil {
		return Result{}, err
	}
	defer os.Remove(file.Name())
	if _, err = io.WriteString(file, input.String()); err != nil {
		file.Close()
		return Result{}, err
	}
	file.Close()
	args = append(args, "--env-file", file.Name(), r.Image)
	if output, createErr := command(ctx, "", nil, "docker", args...); createErr != nil {
		return Result{}, fmt.Errorf("create candidate: %s: %w", strings.TrimSpace(output), createErr)
	}
	success := false
	defer func() {
		if !success {
			cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			command(cleanup, "", nil, "docker", "rm", "-f", name)
		}
	}()
	if output, startErr := command(ctx, "", nil, "docker", "start", name); startErr != nil {
		return Result{}, fmt.Errorf("start candidate: %s: %w", strings.TrimSpace(output), startErr)
	}
	if !strings.HasPrefix(p.HealthPath, "/") || strings.ContainsAny(p.HealthPath, "\r\n") {
		return Result{}, errors.New("invalid health path")
	}
	client := http.Client{Timeout: 3 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	healthy := false
	if p.ServiceType == "worker" {
		if err = processHealth(ctx, name); err != nil {
			return Result{}, err
		}
		healthy = true
	}
	for i := 0; !healthy && i < 30; i++ {
		req, e := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://%s:%d%s", name, p.Port, p.HealthPath), nil)
		if e != nil {
			return Result{}, e
		}
		resp, e := client.Do(req)
		if e == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				healthy = true
				break
			}
		}
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if !healthy {
		return Result{}, errors.New("candidate failed healthcheck")
	}
	if p.Domain != "" {
		if err = a.route(ctx, p.ID, p.Domain, name, p.Port); err != nil {
			if errors.Is(err, errRouteUncertain) {
				success = true
			}
			return Result{}, err
		}
	} else if err = clearRoute(ctx, p.ID); err != nil {
		if errors.Is(err, errRouteUncertain) {
			success = true
		}
		return Result{}, err
	}
	success = true
	backgroundCommitted = true
	if p.ActiveContainer != "" && p.ActiveContainer != name {
		if err = ownedContainer(ctx, p.ActiveContainer, p.ID); err == nil {
			command(ctx, "", nil, "docker", "stop", "--time", "20", p.ActiveContainer)
		}
	}
	message := "HTTP healthcheck passed."
	if p.ServiceType == "worker" {
		message = "Background process remained running for five seconds without restart."
	}
	if p.Domain != "" {
		message += " Caddy route validated and reloaded."
	}
	return Result{Container: name, Image: r.Image, Output: message}, nil
}
func (a *Agent) route(ctx context.Context, id, domain, container string, port int) error {
	file := filepath.Join("/routes", id+".caddy")
	old, readErr := os.ReadFile(file)
	if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}
	address := domain
	if os.Getenv("CADDY_HTTP_ONLY") == "1" {
		address = "http://" + domain
	}
	data := fmt.Sprintf("%s {\n reverse_proxy %s:%d\n}\n", address, container, port)
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, []byte(data), 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, file); err != nil {
		return err
	}
	restore := func() error {
		if old == nil {
			if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
				return err
			}
		} else {
			return os.WriteFile(file, old, 0600)
		}
		return nil
	}
	if _, err := command(ctx, "", nil, "docker", "exec", "deployfox-caddy", "caddy", "validate", "--config", "/etc/caddy/Caddyfile"); err != nil {
		if restoreErr := restore(); restoreErr != nil {
			return errors.Join(errRouteUncertain, restoreErr)
		}
		return errors.New("invalid Caddy config")
	}
	if _, err := command(ctx, "", nil, "docker", "exec", "deployfox-caddy", "caddy", "reload", "--config", "/etc/caddy/Caddyfile"); err != nil {
		if restoreErr := restore(); restoreErr != nil {
			return errors.Join(errRouteUncertain, restoreErr)
		}
		if restoreErr := reloadRestoredRoute(); restoreErr != nil {
			return errors.Join(errRouteUncertain, err, restoreErr)
		}
		return err
	}
	return nil
}
