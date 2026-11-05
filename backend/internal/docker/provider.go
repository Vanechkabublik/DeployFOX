package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"deployfox/internal/config"
	"deployfox/internal/platform"
)

type Request struct {
	GitToken     string            `json:"git_token,omitempty"`
	Backup       string            `json:"backup"`
	CommitSHA    string            `json:"commit_sha"`
	Database     DatabaseSpec      `json:"database"`
	Action       string            `json:"action"`
	Project      platform.Project  `json:"project"`
	DeploymentID string            `json:"deployment_id"`
	Image        string            `json:"image"`
	Environment  map[string]string `json:"environment"`
}
type Result struct {
	Retryable bool   `json:"retryable,omitempty"`
	Error     string `json:"error,omitempty"`
	Container string `json:"container"`
	Image     string `json:"image"`
	Commit    string `json:"commit"`
	Output    string `json:"output"`
}
type Provider struct{ Config config.Config }

func (p Provider) Call(ctx context.Context, request Request) (Result, error) {
	if p.Config.Provider == "mock" {
		return p.mock(ctx, request)
	}
	data, err := json.Marshal(request)
	if err != nil {
		return Result{}, err
	}
	r, err := http.NewRequestWithContext(ctx, "POST", p.Config.AgentURL+"/operations", bytes.NewReader(data))
	if err != nil {
		return Result{}, err
	}
	r.Header.Set("Authorization", "Bearer "+p.Config.AgentToken)
	r.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 35 * time.Minute}).Do(r)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	var result Result
	err = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&result)
	if err == nil && resp.StatusCode != 200 {
		return result, fmt.Errorf("agent: %s", result.Error)
	}
	return result, err
}
func (p Provider) mock(ctx context.Context, r Request) (Result, error) {
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-time.After(500 * time.Millisecond):
	}
	if strings.Contains(r.Project.Source, "fail-deployfox") {
		return Result{}, errors.New("explicit mock failure requested")
	}
	return Result{Container: "mock-" + r.DeploymentID, Image: "mock/" + r.Project.ID + ":" + r.DeploymentID, Commit: "mock", Output: "[MOCK] Lifecycle simulated; no application container exists."}, nil
}
