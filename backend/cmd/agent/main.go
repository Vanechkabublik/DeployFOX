package main

import (
	"log/slog"
	"os"

	"deployfox/internal/docker"
	"deployfox/internal/server"
)

func main() {
	ctx, cancel := server.Context()
	defer cancel()
	token := os.Getenv("AGENT_TOKEN")
	if len(token) < 32 {
		slog.Error("AGENT_TOKEN must contain 32+ characters")
		os.Exit(1)
	}
	if err := server.Serve(ctx, ":8090", &docker.Agent{Token: token}); err != nil {
		slog.Error("agent", "error", err)
		os.Exit(1)
	}
}
