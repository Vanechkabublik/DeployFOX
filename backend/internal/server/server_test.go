package server

import (
	"context"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestGracefulShutdownWaitsForHandlerCleanup(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	done := make(chan error, 1)
	go func() {
		done <- serveListener(ctx, listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(started)
			<-release
			w.WriteHeader(http.StatusNoContent)
		}))
	}()
	go func() {
		response, e := (&http.Client{Timeout: 3 * time.Second}).Get("http://" + listener.Addr().String())
		if e == nil {
			response.Body.Close()
		}
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not start")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("server exited before active handler cleanup")
	case <-time.After(50 * time.Millisecond):
	}
	once.Do(func() { close(release) })
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("graceful shutdown did not finish")
	}
}
