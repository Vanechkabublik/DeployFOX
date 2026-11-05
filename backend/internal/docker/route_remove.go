package docker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

var errRouteUncertain = errors.New("route recovery is uncertain; candidate retained for manual inspection")

func reloadRestoredRoute() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err := command(ctx, "", nil, "docker", "exec", "deployfox-caddy", "caddy", "reload", "--config", "/etc/caddy/Caddyfile")
	return err
}

func clearRoute(ctx context.Context, id string) error {
	file := filepath.Join("/routes", id+".caddy")
	previous, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = os.Remove(file); err != nil {
		return err
	}
	if _, err = command(ctx, "", nil, "docker", "exec", "deployfox-caddy", "caddy", "reload", "--config", "/etc/caddy/Caddyfile"); err != nil {
		if restoreErr := os.WriteFile(file, previous, 0600); restoreErr != nil {
			return errors.Join(errRouteUncertain, err, restoreErr)
		}
		if restoreErr := reloadRestoredRoute(); restoreErr != nil {
			return errors.Join(errRouteUncertain, err, restoreErr)
		}
		return errors.New("cannot clear Caddy route; previous configuration retained")
	}
	return nil
}
