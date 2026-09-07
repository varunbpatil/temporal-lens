// Package http provides the HTTP inbound server for all domains.
package http

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	web "github.com/varunbpatil/temporal-lens"
)

const apiPrefix = "/api"

// Server is the HTTP server that serves all domains.
type Server struct {
	handler  http.Handler
	address  string
	listener net.Listener
	server   *http.Server
	logger   *slog.Logger
	onFatal  func(error)
}

// NewServer creates a new HTTP server that serves the API and embedded UI.
func NewServer(api *http.ServeMux, address string, logger *slog.Logger, onFatal func(error)) *Server {
	return &Server{handler: NewHandler(api), address: address, logger: logger, onFatal: onFatal}
}

// NewHandler serves API routes below /api and the embedded UI at the root.
// Unknown GET and HEAD extensionless paths serve the UI entrypoint for client-side routing.
func NewHandler(api *http.ServeMux) http.Handler {
	ui := http.FileServer(http.FS(embeddedUI()))
	apiHandler := http.StripPrefix(apiPrefix, api)

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, apiPrefix+"/") {
			apiHandler.ServeHTTP(writer, request)
			return
		}
		if request.URL.Path == apiPrefix {
			http.NotFound(writer, request)
			return
		}
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			http.NotFound(writer, request)
			return
		}
		if path.Ext(request.URL.Path) != "" {
			ui.ServeHTTP(writer, request)
			return
		}

		entrypoint := request.Clone(request.Context())
		entrypoint.URL.Path = "/"
		ui.ServeHTTP(writer, entrypoint)
	})
}

func embeddedUI() fs.FS {
	assets, err := fs.Sub(web.Dist, "ui/dist")
	if err == nil {
		_, err = fs.Stat(assets, "index.html")
	}
	if err == nil {
		return assets
	}

	// Non-UI builds embed the placeholder index.html at the root of Dist.
	return web.Dist
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
