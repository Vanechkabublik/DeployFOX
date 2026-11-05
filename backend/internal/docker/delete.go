package docker

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

func (a *Agent) deleteProject(ctx context.Context, r Request) (Result, error) {
	if err := clearRoute(ctx, r.Project.ID); err != nil {
		return Result{}, err
	}
	out, err := command(ctx, "", nil, "docker", "ps", "-aq", "--filter", "label=deployfox.project="+r.Project.ID)
	if err != nil {
		return Result{}, err
	}
	for _, id := range strings.Fields(out) {
		if !regexp.MustCompile(`^[0-9a-f]{12,64}$`).MatchString(id) {
			return Result{}, errors.New("invalid container id")
		}
		if _, err = command(ctx, "", nil, "docker", "rm", "-f", id); err != nil {
			return Result{}, err
		}
	}
	return Result{Output: "Project containers and route removed. Volumes and images retained."}, nil
}
