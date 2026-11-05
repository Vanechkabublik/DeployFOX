package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

type buildOutputKey struct{}
type buildEmitterKey struct{}

type buildEvent struct {
	Line   string  `json:"line,omitempty"`
	Result *Result `json:"result,omitempty"`
}

// Only explicitly selected build tools stream output. Git credential operations,
// rev-parse and Docker inspection must not enter the public build log.
func buildOutputContext(ctx context.Context) context.Context {
	if emit, ok := ctx.Value(buildEmitterKey{}).(func(string)); ok {
		return context.WithValue(ctx, buildOutputKey{}, emit)
	}
	return ctx
}

// exec.Cmd serializes writes when stdout and stderr share the same writer.
// Bound each line before allocating; retain at most 4 KiB even without newlines.
type buildLineWriter struct {
	line []byte
	emit func(string)
}

func (w *buildLineWriter) Write(data []byte) (int, error) {
	for _, b := range data {
		if b == '\n' || b == '\r' {
			w.flush()
		} else if len(w.line) < 4096 {
			w.line = append(w.line, b)
		}
	}
	return len(data), nil
}

func (w *buildLineWriter) flush() {
	if len(w.line) != 0 {
		w.emit(string(w.line))
		w.line = w.line[:0]
	}
}

func (a *Agent) streamBuild(w http.ResponseWriter, r *http.Request, ctx context.Context, request Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	flusher.Flush()
	encoder := json.NewEncoder(w)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	remaining := 1 << 20
	emitted := false
	emit := func(line string) {
		if request.GitToken != "" {
			line = strings.ReplaceAll(line, request.GitToken, "[REDACTED]")
		}
		if remaining <= 0 || ctx.Err() != nil {
			return
		}
		if len(line) > 4096 {
			line = line[:4096]
		}
		if len(line) > remaining {
			line = line[:remaining]
		}
		remaining -= len(line) + 1
		emitted = true
		if encoder.Encode(buildEvent{Line: line}) != nil {
			cancel()
			return
		}
		flusher.Flush()
	}
	ctx = context.WithValue(ctx, buildEmitterKey{}, emit)
	result, err := a.build(ctx, request)
	if err != nil {
		result.Error = err.Error()
	}
	if !emitted && result.Output != "" {
		for _, line := range strings.Split(result.Output, "\n") {
			emit(line)
		}
	}
	// Output already travelled as lines; do not duplicate it in the database.
	result.Output = ""
	if r.Context().Err() == nil {
		_ = encoder.Encode(buildEvent{Result: &result})
		flusher.Flush()
	}
}

func (p Provider) Build(ctx context.Context, request Request, log func(string)) (Result, error) {
	if request.Action != "build" {
		return Result{}, errors.New("streaming is only available for builds")
	}
	if p.Config.Provider == "mock" {
		return p.Call(ctx, request)
	}
	response, err := p.buildResponse(ctx, request)
	if err != nil {
		return Result{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Result{}, errors.New("agent rejected build stream")
	}
	// Agent limits source output to 1 MiB; JSON escaping can expand it sixfold.
	decoder := json.NewDecoder(io.LimitReader(response.Body, 16<<20))
	for {
		var event buildEvent
		if err = decoder.Decode(&event); err != nil {
			return Result{}, errors.New("agent build stream interrupted before final result")
		}
		if event.Line != "" {
			if request.GitToken != "" {
				event.Line = strings.ReplaceAll(event.Line, request.GitToken, "[REDACTED]")
			}
			log(event.Line)
		}
		if event.Result != nil {
			if event.Result.Error != "" {
				return *event.Result, errors.New(event.Result.Error)
			}
			return *event.Result, nil
		}
	}
}

func (p Provider) buildResponse(ctx context.Context, request Request) (*http.Response, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Config.AgentURL+"/operations", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Authorization", "Bearer "+p.Config.AgentToken)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/x-ndjson")
	return (&http.Client{Timeout: 35 * time.Minute}).Do(r)
}
