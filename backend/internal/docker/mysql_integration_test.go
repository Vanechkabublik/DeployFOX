package docker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"deployfox/internal/platform"
	"deployfox/internal/security"
)

func TestRealManagedMySQLRestore(t *testing.T) {
	if os.Getenv("RUN_DOCKER_TESTS") != "1" || os.Getenv("RUN_MYSQL_TESTS") != "1" {
		t.Skip("RUN_DOCKER_TESTS=1 and RUN_MYSQL_TESTS=1 require the MySQL 8.4 image")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	id := security.ID()
	name := "dfdb-" + id
	password := security.Token()
	a := Agent{}
	r := Request{Action: "database-create", Project: platform.Project{ID: id}, Database: DatabaseSpec{Engine: "mysql", Version: "8.4", Password: password}}
	defer func() {
		command(context.Background(), "", nil, "docker", "rm", "-f", name)
		command(context.Background(), "", nil, "docker", "volume", "rm", "dfdb-"+id+"-data")
	}()
	if _, err := a.database(ctx, r); err != nil {
		t.Fatal("MySQL creation/readiness", err)
	}
	sql := func(query string) string {
		t.Helper()
		out, err := command(ctx, "", []string{"MYSQL_PWD=" + password}, "docker", "exec", "-e", "MYSQL_PWD", name, "mysql", "--protocol=tcp", "-h127.0.0.1", "-uapp", "-N", "-B", "-e", query, "app")
		if err != nil {
			t.Fatal("MySQL query failed", err)
		}
		return strings.TrimSpace(out)
	}
	sql("CREATE TABLE recovery_probe(value varchar(64)); INSERT INTO recovery_probe VALUES('survived-mysql')")
	r.Action = "database-backup"
	result, err := a.database(ctx, r)
	if err != nil {
		t.Fatal("MySQL dump", err)
	}
	file := strings.TrimSuffix(result.Output, " (restore verification required)")
	defer os.Remove(file)
	defer os.Remove(file + ".sha256")
	sql("DELETE FROM recovery_probe")
	r.Action = "database-restore"
	r.Backup = filepath.Base(file)
	if _, err = a.database(ctx, r); err != nil {
		t.Fatal("MySQL restore", err)
	}
	if result := sql("SELECT value FROM recovery_probe"); result != "survived-mysql" {
		t.Fatalf("MySQL restore returned %q", result)
	}
}
