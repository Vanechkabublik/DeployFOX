package main

import (
	"log/slog"
	"os"

	"deployfox/internal/config"
	"deployfox/internal/platform"
	"deployfox/internal/server"
)

func main() {
	ctx, cancel := server.Context()
	defer cancel()
	c, err := config.Load()
	if err != nil {
		slog.Error("configuration", "error", err)
		os.Exit(1)
	}
	s, err := platform.Open(ctx, c)
	if err != nil {
		slog.Error("startup", "error", err)
		os.Exit(1)
	}
	defer s.Close()
	if err = server.Serve(ctx, ":8080", server.Router(s)); err != nil {
		slog.Error("server", "error", err)
		os.Exit(1)
	}
}
