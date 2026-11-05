package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type DatabaseSpec struct {
	Engine   string `json:"engine"`
	Version  string `json:"version"`
	Password string `json:"password"`
}

func (a *Agent) database(ctx context.Context, r Request) (Result, error) {
	d := r.Database
	id := r.Project.ID
	name := "dfdb-" + id
	valid := (d.Engine == "postgres" && (d.Version == "16" || d.Version == "17")) || (d.Engine == "redis" && d.Version == "7") || (d.Engine == "mysql" && d.Version == "8.4")
	if !valid || len(d.Password) != 64 || strings.ContainsAny(d.Password, "\n\r\x00") {
		return Result{}, errors.New("invalid database specification")
	}
	if r.Action == "database-create" {
		volume := "dfdb-" + id + "-data"
		if _, err := command(ctx, "", nil, "docker", "volume", "create", "--label", "deployfox.database="+id, volume); err != nil {
			return Result{}, err
		}
		args := []string{"create", "--name", name, "--label", "deployfox.database=" + id, "--network", "deployfox-apps", "--memory", "512m", "--cpus", "1", "--pids-limit", "256", "--security-opt", "no-new-privileges:true", "--restart", "unless-stopped", "--log-opt", "max-size=10m", "--log-opt", "max-file=3"}
		var values, mount string
		switch d.Engine {
		case "postgres":
			mount = "/var/lib/postgresql/data"
			values = "POSTGRES_USER=app\nPOSTGRES_DB=app\nPOSTGRES_PASSWORD=" + d.Password + "\n"
		case "mysql":
			mount = "/var/lib/mysql"
			values = "MYSQL_DATABASE=app\nMYSQL_USER=app\nMYSQL_PASSWORD=" + d.Password + "\nMYSQL_ROOT_PASSWORD=" + d.Password + "\n"
		case "redis":
			mount = "/data"
			values = "REDIS_PASSWORD=" + d.Password + "\n"
		}
		file, err := os.CreateTemp("/work", "db-env-")
		if err != nil {
			return Result{}, err
		}
		defer os.Remove(file.Name())
		if _, err = file.WriteString(values); err != nil {
			file.Close()
			return Result{}, err
		}
		file.Close()
		image := d.Engine + ":" + d.Version
		if d.Engine == "postgres" || d.Engine == "redis" {
			image += "-alpine"
		}
		args = append(args, "--mount", "type=volume,source="+volume+",target="+mount, "--env-file", file.Name(), image)
		if d.Engine == "redis" {
			args = append(args, "sh", "-c", `exec redis-server --appendonly yes --requirepass "$REDIS_PASSWORD"`)
		}
		if _, err = command(ctx, "", nil, "docker", args...); err != nil {
			return Result{}, err
		}
		if _, err = command(ctx, "", nil, "docker", "start", name); err != nil {
			return Result{}, err
		}
		if err = databaseHealth(ctx, name, d); err != nil {
			return Result{}, err
		}
		return Result{Output: "Database passed readiness check; persistent volume retained."}, nil
	}
	out, err := command(ctx, "", nil, "docker", "inspect", "--format", `{{index .Config.Labels "deployfox.database"}}`, name)
	if err != nil || strings.TrimSpace(out) != id {
		return Result{}, errors.New("database ownership mismatch")
	}
	if r.Action == "database-backup" {
		if err = os.MkdirAll("/backups", 0700); err != nil {
			return Result{}, err
		}
		ext := "dump"
		if d.Engine == "mysql" {
			ext = "sql"
		}
		if d.Engine == "redis" {
			ext = "rdb"
		}
		filename := fmt.Sprintf("/backups/%s-%s.%s", id, time.Now().UTC().Format("20060102T150405"), ext)
		file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return Result{}, err
		}
		defer file.Close()
		var cmd *exec.Cmd
		if d.Engine == "redis" {
			remote := "/tmp/" + id + ".rdb"
			if _, err = command(ctx, "", []string{"REDISCLI_AUTH=" + d.Password}, "docker", "exec", "-e", "REDISCLI_AUTH", name, "redis-cli", "--rdb", remote); err != nil {
				os.Remove(filename)
				return Result{}, errors.New("redis snapshot failed")
			}
			if _, err = command(ctx, "", nil, "docker", "cp", name+":"+remote, filename); err != nil {
				os.Remove(filename)
				return Result{}, err
			}
			command(ctx, "", nil, "docker", "exec", name, "rm", "-f", remote)
			if err = os.Chmod(filename, 0600); err != nil {
				return Result{}, err
			}
			if err = backupChecksum(filename); err != nil {
				return Result{}, err
			}
			return Result{Output: filename + " (restore verification required)"}, nil
		}
		if d.Engine == "postgres" {
			cmd = exec.CommandContext(ctx, "docker", "exec", name, "pg_dump", "-U", "app", "-d", "app", "-Fc")
		} else {
			cmd = exec.CommandContext(ctx, "docker", "exec", "-e", "MYSQL_PWD", name, "mysqldump", "-uapp", "--single-transaction", "--no-tablespaces", "app")
			cmd.Env = append(os.Environ(), "MYSQL_PWD="+d.Password)
		}
		cmd.Stdout = file
		var stderr boundedBuffer
		cmd.Stderr = &stderr
		if err = cmd.Run(); err != nil {
			os.Remove(filename)
			return Result{}, errors.New("database dump failed")
		}
		if err = file.Sync(); err != nil {
			return Result{}, err
		}
		if err = backupChecksum(filename); err != nil {
			return Result{}, err
		}
		return Result{Output: filename + " (restore verification required)"}, nil
	}
	if r.Action == "database-restore" {
		if d.Engine == "redis" {
			return Result{}, errors.New("redis restore requires a documented maintenance procedure; panel restore supports PostgreSQL and MySQL")
		}
		pattern := `^` + regexp.QuoteMeta(id) + `-[0-9]{8}T[0-9]{6}\.(dump|sql)$`
		if !regexp.MustCompile(pattern).MatchString(r.Backup) {
			return Result{}, errors.New("invalid backup filename")
		}
		if (d.Engine == "postgres" && !strings.HasSuffix(r.Backup, ".dump")) || (d.Engine == "mysql" && !strings.HasSuffix(r.Backup, ".sql")) {
			return Result{}, errors.New("backup format does not match database")
		}
		filename, err := containedPath("/backups", r.Backup)
		if err != nil {
			return Result{}, err
		}
		if filepath.Dir(filename) != "/backups" {
			return Result{}, errors.New("backup must be in managed backup directory")
		}
		file, err := os.Open(filename)
		if err != nil {
			return Result{}, err
		}
		defer file.Close()
		hash := sha256.New()
		if _, err = io.Copy(hash, file); err != nil {
			return Result{}, err
		}
		expected, err := os.ReadFile(filename + ".sha256")
		if err != nil || strings.TrimSpace(string(expected)) != hex.EncodeToString(hash.Sum(nil)) {
			return Result{}, errors.New("backup checksum mismatch or missing")
		}
		if _, err = file.Seek(0, io.SeekStart); err != nil {
			return Result{}, err
		}
		var cmd *exec.Cmd
		if d.Engine == "postgres" {
			cmd = exec.CommandContext(ctx, "docker", "exec", "-i", name, "pg_restore", "-U", "app", "-d", "app", "--clean", "--if-exists", "--exit-on-error")
		} else {
			cmd = exec.CommandContext(ctx, "docker", "exec", "-i", "-e", "MYSQL_PWD", name, "mysql", "-uapp", "app")
			cmd.Env = append(os.Environ(), "MYSQL_PWD="+d.Password)
		}
		cmd.Stdin = file
		var output boundedBuffer
		cmd.Stdout = &output
		cmd.Stderr = &output
		if err = cmd.Run(); err != nil {
			return Result{}, errors.New("restore failed; database may be partially restored; inspect before restarting applications")
		}
		return Result{Output: "Database restored. Verify application data before resuming traffic."}, nil
	}
	action := strings.TrimPrefix(r.Action, "database-")
	if action != "start" && action != "stop" && action != "restart" {
		return Result{}, errors.New("invalid database action")
	}
	_, err = command(ctx, "", nil, "docker", action, name)
	if err != nil {
		return Result{}, err
	}
	if action != "stop" {
		if err = databaseHealth(ctx, name, d); err != nil {
			return Result{}, err
		}
	}
	return Result{Output: "Database operation completed"}, err
}

func backupChecksum(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return err
	}
	return os.WriteFile(filename+".sha256", []byte(hex.EncodeToString(hash.Sum(nil))+"\n"), 0600)
}
