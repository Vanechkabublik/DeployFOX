package docker

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"deployfox/internal/platform"
	"deployfox/internal/security"
)

func TestRealManagedRedisBackup(t *testing.T) {
	if os.Getenv("RUN_DOCKER_TESTS") != "1" {
		t.Skip("explicit RUN_DOCKER_TESTS=1 required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	id := security.ID()
	name := "dfdb-" + id
	password := security.Token()
	a := Agent{}
	r := Request{Action: "database-create", Project: platform.Project{ID: id}, Database: DatabaseSpec{Engine: "redis", Version: "7", Password: password}}
	defer func() {
		command(context.Background(), "", nil, "docker", "rm", "-f", name)
		command(context.Background(), "", nil, "docker", "volume", "rm", "dfdb-"+id+"-data")
	}()
	if _, err := a.database(ctx, r); err != nil {
		t.Fatal(err)
	}
	if out, err := command(ctx, "", []string{"REDISCLI_AUTH=" + password}, "docker", "exec", "-e", "REDISCLI_AUTH", name, "redis-cli", "SET", "recovery_probe", "survived-redis"); err != nil || strings.TrimSpace(out) != "OK" {
		t.Fatal("Redis write failed", err)
	}
	r.Action = "database-backup"
	result, err := a.database(ctx, r)
	if err != nil {
		t.Fatal("Redis RDB export", err)
	}
	filename := strings.TrimSuffix(result.Output, " (restore verification required)")
	defer os.Remove(filename)
	defer os.Remove(filename + ".sha256")
	probe := "redis-restore-" + id
	volume := "redis-restore-" + id
	defer func() {
		command(context.Background(), "", nil, "docker", "rm", "-f", probe)
		command(context.Background(), "", nil, "docker", "volume", "rm", volume)
	}()
	if _, err = command(ctx, "", nil, "docker", "volume", "create", volume); err != nil {
		t.Fatal(err)
	}
	if out, err := command(ctx, "", nil, "docker", "create", "--name", probe, "--network", "none", "--mount", "type=volume,source="+volume+",target=/data", "redis:7-alpine", "redis-server", "--appendonly", "no"); err != nil {
		t.Fatal("Redis restore probe creation", out, err)
	}
	if _, err = command(ctx, "", nil, "docker", "cp", filename, probe+":/data/dump.rdb"); err != nil {
		t.Fatal(err)
	}
	if _, err = command(ctx, "", nil, "docker", "start", probe); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		out, err := command(ctx, "", nil, "docker", "exec", probe, "redis-cli", "GET", "recovery_probe")
		if err == nil && strings.TrimSpace(out) == "survived-redis" {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("RDB did not restore the expected Redis value")
}
