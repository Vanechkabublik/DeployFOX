package docker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"deployfox/internal/config"
	"deployfox/internal/platform"
	"deployfox/internal/security"
)

func TestBuildOutputArrivesBeforeProcessExit(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("POSIX shell required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	line := make(chan string, 4)
	ctx = context.WithValue(ctx, buildOutputKey{}, func(value string) { line <- value })
	done := make(chan error, 1)
	go func() {
		_, err := command(ctx, "", nil, "sh", "-c", "printf 'first\\n'; sleep 1; printf 'last'")
		done <- err
	}()
	select {
	case first := <-line:
		if first != "first" {
			t.Fatal(first)
		}
	case <-done:
		t.Fatal("output was buffered until command exit")
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case <-done:
		t.Fatal("process exited before first line was consumed")
	default:
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if last := <-line; last != "last" {
		t.Fatal("unterminated final line was lost", last)
	}
}

func TestBuildProviderConsumesLiveStreamAndRequiresFinalResult(t *testing.T) {
	for _, complete := range []bool{false, true} {
		received := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer test-agent" || r.Header.Get("Accept") != "application/x-ndjson" {
				t.Error("missing agent authentication or stream negotiation")
			}
			w.Header().Set("Content-Type", "application/x-ndjson")
			encoder := json.NewEncoder(w)
			_ = encoder.Encode(buildEvent{Line: "building token-secret"})
			w.(http.Flusher).Flush()
			select {
			case <-received:
			case <-r.Context().Done():
				return
			}
			if complete {
				_ = encoder.Encode(buildEvent{Result: &Result{Image: "built-image"}})
			}
		}))
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		provider := Provider{Config: config.Config{AgentURL: server.URL, AgentToken: "test-agent", Provider: "docker"}}
		result, err := provider.Build(ctx, Request{Action: "build", GitToken: "token-secret"}, func(line string) {
			if strings.Contains(line, "token-secret") {
				t.Error("GitHub token leaked into live output")
			}
			close(received)
		})
		cancel()
		server.Close()
		if complete && (err != nil || result.Image != "built-image") {
			t.Fatal("missing final result", result, err)
		}
		if !complete && err == nil {
			t.Fatal("truncated stream reported success")
		}
	}
}

func TestBuildLineBufferIsBounded(t *testing.T) {
	var lines []string
	writer := buildLineWriter{emit: func(line string) { lines = append(lines, line) }}
	_, _ = writer.Write([]byte(strings.Repeat("a", 1<<20) + "\nnext\r"))
	writer.flush()
	if len(lines) != 2 || len(lines[0]) != 4096 || lines[1] != "next" {
		t.Fatal("line framing or memory bound failed")
	}
}

func TestAgentNegotiatesBuildStream(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("POSIX shell required")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$1\" in pull) printf 'pull-progress\\n';; tag) exit 0;; *) exit 1;; esac\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	server := httptest.NewServer(&Agent{Token: "test-agent"})
	defer server.Close()
	provider := Provider{Config: config.Config{Provider: "docker", AgentURL: server.URL, AgentToken: "test-agent"}}
	request := Request{Action: "build", DeploymentID: security.ID(), Project: platform.Project{ID: security.ID(), SourceType: "image", Source: "alpine:3.22"}}
	var lines []string
	result, err := provider.Build(context.Background(), request, func(line string) { lines = append(lines, line) })
	if err != nil || result.Image == "" || result.Output != "" || len(lines) != 1 || lines[0] != "pull-progress" {
		t.Fatal("agent streaming protocol failed", result, lines, err)
	}
}
