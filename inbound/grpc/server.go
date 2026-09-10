// Package grpc provides the gRPC inbound server for all domains.
package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"connectrpc.com/connect"
)

// Server is the gRPC server that serves all domains.
type Server struct {
	mux      *http.ServeMux
	address  string
	listener net.Listener
	server   *http.Server
	logger   *slog.Logger
	onFatal  func(error)
}

// NewServer creates a new gRPC server with compression configured.
func NewServer(address string, logger *slog.Logger, onFatal func(error)) *Server {
	return &Server{
		mux:     http.NewServeMux(),
		address: address,
		logger:  logger,
		onFatal: onFatal,
	}
}

// Mux returns the underlying ServeMux for domain handlers to register on.
func (s *Server) Mux() *http.ServeMux {
	return s.mux
}

// HandlerOptions returns the default handler options (compression, error
// logging, etc.) that domain handlers should use when registering.
func (s *Server) HandlerOptions() []connect.HandlerOption {
	options := HandlerOptions()
	return append(options, connect.WithInterceptors(loggingInterceptor(s.logger)))
}

// HandlerOptions returns handler options that do not depend on a server instance.
// Use Server.HandlerOptions when request error logging is required.
func HandlerOptions() []connect.HandlerOption {
	return []connect.HandlerOption{
		connect.WithCompression(
			Brotli,
			NewBrotliDecompressor,
			NewBrotliCompressor,
		),
	}
}

func loggingInterceptor(logger *slog.Logger) connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
			response, err := next(ctx, request)
			if err != nil && logger != nil {
				logger.ErrorContext(ctx, "RPC failed",
					"procedure", request.Spec().Procedure,
					"code", connect.CodeOf(err),
					"error", err,
				)
			}
			return response, err
		}
	})
}

// Start begins listening for connections. It returns once the server is accepting connections.
// The server runs in a background goroutine and is shut down when Stop is called.
func (s *Server) Start(ctx context.Context) error {
	var lc net.ListenConfig
	var err error
	s.listener, err = lc.Listen(context.Background(), "tcp", s.address)
	if err != nil {
		return fmt.Errorf("grpc: listen: %w", err)
	}

	protocols := &http.Protocols{}
	protocols.SetHTTP1(false)
	protocols.SetUnencryptedHTTP2(true)

	s.server = &http.Server{
		Addr:              s.address,
		Handler:           s.mux,
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
