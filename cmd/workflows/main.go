package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/varunbpatil/temporal-lens/config"
	"github.com/varunbpatil/temporal-lens/version"
)

func main() {
	// Configuration
	cfg, err := config.Parse()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Logging
	logger := slog.New(newHandler(cfg.Log))
	slog.SetDefault(logger)

	// Version info
	logger.Info("starting temporal-lens",
		"version", version.Version,
		"commit", version.GitCommit,
		"build_time", version.BuildTime,
	)

	// Repository adapters
	// OpenSearch, Temporal, etc.

	// Domain services
	// Workflow service, etc.

	// Inbound adapters
	// gRPC, HTTP, MCP servers, etc.

	// Start server
	const sleepDuration = 3600 * time.Second
	time.Sleep(sleepDuration)
}

func newHandler(cfg config.LogConfig) slog.Handler {
	opts := &slog.HandlerOptions{
		Level: parseLevel(cfg.Level),
	}

	switch cfg.Format {
	case "json":
		return slog.NewJSONHandler(os.Stderr, opts)
	default:
		return slog.NewTextHandler(os.Stderr, opts)
	}
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
