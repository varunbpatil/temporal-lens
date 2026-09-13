package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/varunbpatil/temporal-lens/config"
	workflowsService "github.com/varunbpatil/temporal-lens/domains/workflows/service"
	grpcserver "github.com/varunbpatil/temporal-lens/inbound/grpc"
	grpcworkflows "github.com/varunbpatil/temporal-lens/inbound/grpc/workflows"
	httpserver "github.com/varunbpatil/temporal-lens/inbound/http"
	mcpserver "github.com/varunbpatil/temporal-lens/inbound/mcp"
	mcpworkflows "github.com/varunbpatil/temporal-lens/inbound/mcp/workflows"
	"github.com/varunbpatil/temporal-lens/mapper"
	workflowsRepository "github.com/varunbpatil/temporal-lens/outbound/opensearch/workflows"
	workflowsSource "github.com/varunbpatil/temporal-lens/outbound/temporal/workflows"
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
	logger := slog.New(newLogHandler(cfg.Log))
	slog.SetDefault(logger)

	// Version info
	logger.Info("starting temporal-lens",
		"version", version.Version,
		"commit", version.GitCommit,
		"build_time", version.BuildTime,
	)

	// Context with signal cancellation
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	onFatal := func(err error) { logger.Error("fatal service error", "error", err); cancel() }

	// Lifecycle manager
	lm := types.NewManager(logger)
	defer func() { cancel(); lm.StopAll(shutdownTimeout) }()

	// Temporal workflow source
	source, err := workflowsSource.New(ctx, workflowsSource.WorkflowSourceParams{Config: cfg.Temporal})
	if err != nil {
		logger.Error("create Temporal workflow source", "error", err)
		return 1
	}
	lm.AddCloser("Temporal workflow source", source)

	var workflowSvc *workflowsService.Service

	// Temporal workflow repository
	repository, err := workflowsRepository.New(ctx, workflowsRepository.WorkflowRepositoryParams{
		Config: cfg.OpenSearch,

		// Repository needs the schema from the workflow service,
		// but the workflow service itself needs the repository.
		// This closure is a way of overcoming that circular dependency.
		SearchSchema: func() types.Schema { return workflowSvc.SearchSchemas(context.Background()).Combined() },
	})
	if err != nil {
		logger.Error("create OpenSearch workflow repository", "error", err)
		return 1
	}
	lm.AddCloser("OpenSearch workflow repository", repository)

	// Workflow service
	workflowSvc, err = workflowsService.New(ctx, workflowsService.WorkflowServiceParams{
		Config:     cfg.Temporal,
		Source:     source,
		Repository: repository,
		Logger:     logger,
		Mapper:     mapper.Mapper{},
	})
	if err != nil {
		logger.Error("create workflow service", "error", err)
		return 1
	}
	lm.Add("Workflows service", workflowSvc)

	// gRPC adapter
	grpcSrv := grpcserver.New(cfg.GRPC.Address, logger, onFatal)
	grpcworkflows.Register(grpcSrv, grpcworkflows.New(workflowSvc, cfg.ReadOnly))
	lm.Add("gRPC", grpcSrv)

	// MCP adapter
	mcpSrv := mcpserver.New()
	mcpworkflows.Register(mcpSrv, workflowSvc, cfg.ReadOnly)

	// HTTP adapter
	httpSrv := httpserver.New(grpcSrv.Handler(), mcpserver.Handler(mcpSrv), cfg.HTTP.Address, logger, onFatal)
	lm.Add("HTTP", httpSrv)

	// Start all services
	if startErr := lm.StartAll(ctx); startErr != nil {
		logger.Error("startup failed", "error", startErr)
		return 1
	}

	// Shutdown gracefully
	<-ctx.Done()
	return 0
}

func newLogHandler(cfg config.LogConfig) slog.Handler {
	opts := &slog.HandlerOptions{Level: parseLevel(cfg.Level)}

	switch strings.ToLower(cfg.Format) {
	case "json":
		return slog.NewJSONHandler(os.Stderr, opts)
	default:
		return slog.NewTextHandler(os.Stderr, opts)
	}
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
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
