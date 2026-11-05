//go:build !linux

package docker

import (
	"context"
	"errors"
)

func hostMetrics(context.Context) (Result, error) {
	return Result{}, errors.New("host metrics require Linux agent")
}
