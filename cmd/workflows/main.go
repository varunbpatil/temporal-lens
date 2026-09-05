package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/varunbpatil/temporal-lens/config"
	grpcserver "github.com/varunbpatil/temporal-lens/inbound/grpc"
	grpcworkflows "github.com/varunbpatil/temporal-lens/inbound/grpc/workflows"
	httpserver "github.com/varunbpatil/temporal-lens/inbound/http"
	"github.com/varunbpatil/temporal-lens/types"
	"github.com/varunbpatil/temporal-lens/version"
)

const shutdownTimeout = 10 * time.Second

func main() {
	os.Exit(run())
}

func run() int {
	// Configuration
	cfg, err := config.Parse()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
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

	// Context with signal cancellation
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	onFatal := func(err error) {
		logger.Error("fatal service error", "error", err)
		cancel()
	}

	// Lifecycle manager
	lm := types.NewManager(logger)

	// Inbound adapters
	grpcSrv := grpcserver.NewServer(cfg.GRPC.Address, logger, onFatal)
	grpcworkflows.Register(grpcSrv.Mux(), grpcworkflows.NewHandler(nil))
	lm.Add("grpc", grpcSrv)

	httpSrv := httpserver.NewServer(grpcSrv.Mux(), cfg.HTTP.Address, logger, onFatal)
	lm.Add("http", httpSrv)

	// Start all services
	if startErr := lm.StartAll(ctx); startErr != nil {
		logger.Error("startup failed", "error", startErr)

		// Stop earlier services
		cancel()
		lm.StopAll(shutdownTimeout)

		return 1
	}

	// Shutdown gracefully
	<-ctx.Done()
	lm.StopAll(shutdownTimeout)
	return 0
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
