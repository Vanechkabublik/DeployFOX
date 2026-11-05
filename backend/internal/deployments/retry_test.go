package deployments

import (
	"context"
	"errors"
	"testing"

	"deployfox/internal/docker"
)

type failingBuild struct {
	calls     int
	transient bool
}

func (f *failingBuild) Call(context.Context, docker.Request) (docker.Result, error) {
	f.calls++
	return docker.Result{Retryable: f.transient}, errors.New("failure")
}

func TestBuildRetriesAreBoundedAndPermanentErrorsAreNotRetried(t *testing.T) {
	for _, temporary := range []bool{false, true} {
		provider := &failingBuild{transient: temporary}
		retries := 0
		_, err := retryBuild(context.Background(), provider, docker.Request{Action: "build"}, func() error { retries++; return nil }, func(string) {})
		want := 1
		if temporary {
			want = 3
		}
		if err == nil || provider.calls != want || retries != want-1 {
			t.Fatalf("transient=%v calls=%d retries=%d", temporary, provider.calls, retries)
		}
	}
}
func TestCancelledBuildDoesNotStartOrRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider := &failingBuild{transient: true}
	_, err := retryBuild(ctx, provider, docker.Request{Action: "build"}, func() error { return nil }, func(string) {})
	if !errors.Is(err, context.Canceled) || provider.calls != 0 {
		t.Fatal("cancelled build executed")
	}
}
