// Package grpc provides the gRPC inbound server for all domains.
package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/varunbpatil/temporal-lens/inbound/observability"
)

// Server is the gRPC server that serves all domains.
type Server struct {
	mux      *http.ServeMux
	handler  http.Handler
	address  string
	listener net.Listener
	server   *http.Server
	logger   *slog.Logger
	onFatal  func(error)
}

// Registrar registers configured Connect handlers with a transport.
// Domain-specific inbound adapters depend on this narrow interface rather than
// on Server or an underlying HTTP mux.
type Registrar interface {
	Handle(string, http.Handler)
	HandlerOptions() []connect.HandlerOption
}

// New creates a new gRPC server with compression configured.
func New(address string, logger *slog.Logger, onFatal func(error)) *Server {
	mux := http.NewServeMux()
	server := &Server{
		mux:     mux,
		address: address,
		logger:  logger,
		onFatal: onFatal,
	}
	var handler http.Handler = mux
	handler = observability.Recoverer(logger)(handler)
	handler = observability.RequestID(handler)
	server.handler = handler
	return server
}

// Handler returns the Connect handler with request IDs and panic recovery.
// It can be mounted behind another HTTP router or served directly.
func (s *Server) Handler() http.Handler {
	return s.handler
}

// Handle registers a Connect handler. Domain adapters should use this method
// instead of accessing the underlying mux.
func (s *Server) Handle(pattern string, handler http.Handler) {
	s.mux.Handle(pattern, handler)
}

// HandlerOptions returns the default handler options (compression, error
// logging, etc.) that domain handlers should use when registering.
func (s *Server) HandlerOptions() []connect.HandlerOption {
	return []connect.HandlerOption{
		connect.WithCompression(Brotli, NewBrotliDecompressor, NewBrotliCompressor),
		connect.WithInterceptors(newLoggingInterceptor(s.logger)),
	}
}

type loggingInterceptor struct {
	logger *slog.Logger
}

func newLoggingInterceptor(logger *slog.Logger) loggingInterceptor {
	return loggingInterceptor{logger: logger}
}

func (i loggingInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
		start := time.Now()
		response, err := next(ctx, request)
		i.log(ctx, request.Spec(), request.Header(), start, err)
		return response, err
	}
}

func (loggingInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	// no-op because this interceptor is installed only on inbound server handlers.
	// Outbound Connect clients should use their own observability interceptor.
	return next
}

func (i loggingInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, connection connect.StreamingHandlerConn) error {
		start := time.Now()
		err := next(ctx, connection)
		i.log(ctx, connection.Spec(), connection.RequestHeader(), start, err)
		return err
	}
}

func (i loggingInterceptor) log(
	ctx context.Context,
	spec connect.Spec,
	header http.Header,
	start time.Time,
	err error,
) {
	if i.logger == nil {
		return
	}
	attrs := []any{
		"procedure", spec.Procedure,
		"stream_type", spec.StreamType.String(),
		"duration", time.Since(start),
	}
	if requestID := strings.TrimSpace(header.Get(observability.RequestIDHeader)); requestID != "" {
		attrs = append(attrs, "request_id", requestID)
	}
	if err != nil {
		attrs = append(attrs, "code", connect.CodeOf(err), "error", err)
		i.logger.ErrorContext(ctx, "RPC failed", attrs...)
		return
	}
	i.logger.InfoContext(ctx, "RPC completed", attrs...)
}

// Start begins listening for connections. The server runs in a background goroutine and is shut down when Stop is called.
func (s *Server) Start(ctx context.Context) error {
	var lc net.ListenConfig
	var err error
	s.listener, err = lc.Listen(ctx, "tcp", s.address)
	if err != nil {
		return fmt.Errorf("grpc: listen: %w", err)
	}

	// gRPC clients speak HTTP/2, so serve it over plaintext (h2c): Go enables
	// HTTP/2 only for TLS by default, and HTTP/1.1 would make the port useless
	// for gRPC. Disabling HTTP/1 makes the port a dedicated gRPC endpoint.
	protocols := &http.Protocols{}
	protocols.SetHTTP1(false)
	protocols.SetUnencryptedHTTP2(true)

	s.server = &http.Server{
		Addr:              s.address,
		Handler:           s.handler,
		Protocols:         protocols,
		ReadHeaderTimeout: 10 * time.Second, //nolint:mnd // reasonable default
	}

	go func() {
		serveErr := s.server.Serve(s.listener)
		if serveErr != nil && serveErr != http.ErrServerClosed {
			s.logger.ErrorContext(ctx, "grpc serve error", "error", serveErr)
			if s.onFatal != nil {
				s.onFatal(serveErr)
			}
		}
	}()

	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}
