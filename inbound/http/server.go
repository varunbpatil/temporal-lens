// Package http provides the HTTP inbound server for all domains.
package http

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Server is the HTTP server that serves all domains.
type Server struct {
	handler  http.Handler
	address  string
	listener net.Listener
	server   *http.Server
	logger   *slog.Logger
	onFatal  func(error)
}

// NewServer creates a new HTTP server that wraps an existing handler (typically a gRPC mux).
func NewServer(handler http.Handler, address string, logger *slog.Logger, onFatal func(error)) *Server {
	return &Server{handler: handler, address: address, logger: logger, onFatal: onFatal}
}

// Start begins listening for connections. It returns once the server is accepting connections.
// The server runs in a background goroutine and is shut down when Stop is called.
func (s *Server) Start(ctx context.Context) error {
	var lc net.ListenConfig
	var err error
	s.listener, err = lc.Listen(context.Background(), "tcp", s.address)
	if err != nil {
		return fmt.Errorf("http: listen: %w", err)
	}

	s.server = &http.Server{
		Addr:              s.address,
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second, //nolint:mnd // reasonable default
	}

	go func() {
		serveErr := s.server.Serve(s.listener)
		if serveErr != nil && serveErr != http.ErrServerClosed {
			s.logger.ErrorContext(ctx, "http serve error", "error", serveErr)
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
