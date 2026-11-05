package docker

import (
	"context"
	"errors"
	"strings"
	"time"
)

func processHealth(ctx context.Context, name string) error {
	for i := 0; i < 5; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
		out, err := command(ctx, "", nil, "docker", "inspect", "--format", "{{.State.Running}} {{.RestartCount}}", name)
		if err != nil || strings.TrimSpace(out) != "true 0" {
			return errors.New("background process exited or restarted during startup")
		}
	}
	return nil
}
