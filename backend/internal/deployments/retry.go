package deployments

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"deployfox/internal/docker"
)

type buildCaller interface {
	Call(context.Context, docker.Request) (docker.Result, error)
}

type streamingBuildCaller interface {
	Build(context.Context, docker.Request, func(string)) (docker.Result, error)
}

var errBuildCancelled = errors.New("build cancellation requested at retry boundary")

// Only image construction is retried; build commands must avoid external mutations.
// Traffic changes are never blindly replayed.
func retryBuild(ctx context.Context, provider buildCaller, request docker.Request, onRetry func() error, log func(string)) (docker.Result, error) {
	if request.Action != "build" {
		return docker.Result{}, errors.New("retry is only allowed for image construction")
	}
	var result docker.Result
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if streaming, ok := provider.(streamingBuildCaller); ok {
			result, err = streaming.Build(ctx, request, log)
		} else {
			result, err = provider.Call(ctx, request)
		}
		if request.GitToken != "" {
			result.Output = strings.ReplaceAll(result.Output, request.GitToken, "[REDACTED]")
		}
		for _, line := range strings.Split(result.Output, "\n") {
			if line != "" {
				log(line)
			}
		}
		if err == nil {
			return result, nil
		}
		var network net.Error
		if attempt == 2 || (!result.Retryable && !errors.As(err, &network)) || ctx.Err() != nil {
			return result, err
		}
		if err = onRetry(); err != nil {
			return result, err
		}
		log("Temporary build failure; retrying before any traffic change")
		timer := time.NewTimer(time.Duration(1<<attempt) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return result, ctx.Err()
		case <-timer.C:
		}
	}
	return result, err
}
