package docker

import (
	"context"
	"errors"
	"strings"
	"time"
)

func databaseHealth(ctx context.Context, name string, d DatabaseSpec) error {
	for attempt := 0; attempt < 60; attempt++ {
		var out string
		var err error
		switch d.Engine {
		case "postgres":
			_, err = command(ctx, "", nil, "docker", "exec", name, "pg_isready", "-h", "127.0.0.1", "-U", "app", "-d", "app")
		case "mysql":
			out, err = command(ctx, "", []string{"MYSQL_PWD=" + d.Password}, "docker", "exec", "-e", "MYSQL_PWD", name, "mysql", "--protocol=tcp", "-h127.0.0.1", "-uapp", "-N", "-e", "SELECT 1", "app")
			if strings.TrimSpace(out) != "1" {
				err = errors.New("MySQL not ready")
			}
		case "redis":
			out, err = command(ctx, "", []string{"REDISCLI_AUTH=" + d.Password}, "docker", "exec", "-e", "REDISCLI_AUTH", name, "redis-cli", "ping")
			if strings.TrimSpace(out) != "PONG" {
				err = errors.New("redis not ready")
			}
		}
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return errors.New("database readiness timed out; volume retained")
}
