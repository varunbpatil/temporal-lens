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
	workflowservice "github.com/varunbpatil/temporal-lens/domains/workflows/service"
	grpcserver "github.com/varunbpatil/temporal-lens/inbound/grpc"
	grpcworkflows "github.com/varunbpatil/temporal-lens/inbound/grpc/workflows"
	httpserver "github.com/varunbpatil/temporal-lens/inbound/http"
	"github.com/varunbpatil/temporal-lens/mapper"
	opensearchworkflows "github.com/varunbpatil/temporal-lens/outbound/opensearch/workflows"
	temporalworkflows "github.com/varunbpatil/temporal-lens/outbound/temporal/workflows"
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
	onFatal := func(err error) { logger.Error("fatal service error", "error", err); cancel() }

	// Lifecycle manager
	lm := types.NewManager(logger)
	defer func() { cancel(); lm.StopAll(shutdownTimeout) }()

	// Temporal workflow source
	source, err := temporalworkflows.NewSource(ctx, temporalworkflows.WorkflowSourceParams{Config: cfg.Temporal})
	if err != nil {
		logger.Error("create Temporal workflow source", "error", err)
		return 1
	}
	lm.Add("Temporal workflow source", types.CloseOnly(source.Close))

	var workflowSvc *workflowservice.Service

	// Temporal workflow repository
	repository, err := opensearchworkflows.NewRepository(ctx, opensearchworkflows.WorkflowRepositoryParams{
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
	lm.Add("OpenSearch workflow repository", types.CloseOnly(repository.Close))

	// Workflow service
	workflowSvc, err = workflowservice.NewService(ctx, workflowservice.WorkflowServiceParams{
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
	grpcSrv := grpcserver.NewServer(cfg.GRPC.Address, logger, onFatal)
	grpcworkflows.Register(grpcSrv.Mux(), grpcworkflows.NewHandler(workflowSvc), grpcSrv.HandlerOptions()...)
	lm.Add("gRPC", grpcSrv)

	// HTTP adapter
	httpSrv := httpserver.NewServer(grpcSrv.Mux(), cfg.HTTP.Address, logger, onFatal)
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

func newHandler(cfg config.LogConfig) slog.Handler {
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
