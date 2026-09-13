// Package http provides the HTTP inbound server for all domains.
package http

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httplog/v3"
	"github.com/vearutop/statigz"
	"github.com/vearutop/statigz/brotli"

	web "github.com/varunbpatil/temporal-lens"
	"github.com/varunbpatil/temporal-lens/inbound/observability"
)

const (
	apiPrefix = "/api"
	mcpPrefix = "/mcp"
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

// New creates a new HTTP server that serves the API, MCP and embedded UI.
func New(api, mcp http.Handler, address string, logger *slog.Logger, onFatal func(error)) *Server {
	return &Server{
		handler: newHandler(api, mcp, logger),
		address: address,
		logger:  logger,
		onFatal: onFatal,
	}
}

// newHandler serves Connect API routes below /api, MCP at /mcp, and the
// embedded UI at the root. Browser navigation falls back to the UI entrypoint;
// missing static assets return 404. Connect RPCs own their completion logs,
// MCP access is logged via httplog, and the UI stays quiet.
func newHandler(api, mcp http.Handler, logger *slog.Logger) http.Handler {
	ui := newUIHandler(embeddedUI())
	router := chi.NewRouter()

	// Global middleware
	router.Use(observability.RequestID)
	router.Use(observability.Recoverer(logger))

	// MCP: the endpoint for AI agents. Access-logged via httplog.
	router.With(httplog.RequestLogger(logger, &httplog.Options{
		LogExtraAttrs: requestLogAttrs,
	})).Handle(mcpPrefix, mcp)

	// API: Connect RPCs below /api. Brotli is put first in Accept-Encoding
	// because Connect picks the first mutually-supported encoding.
	router.Route(apiPrefix, func(router chi.Router) {
		router.Use(preferBrotli)
		router.Handle("/", http.NotFoundHandler())
		router.Handle("/*", http.StripPrefix(apiPrefix, api))
	})

	// UI: embedded SPA at the root. Missing static assets return 404;
	// extensionless paths fall back to the entrypoint.
	router.Get("/*", ui.ServeHTTP)
	router.Head("/*", ui.ServeHTTP)

	return router
}

func requestLogAttrs(request *http.Request, _ string, _ int) []slog.Attr {
	if requestID := observability.RequestIDFromContext(request.Context()); requestID != "" {
		return []slog.Attr{slog.String("request_id", requestID)}
	}
	return nil
}

// preferBrotli moves Brotli to the front of Accept-Encoding for Connect API
// requests. Connect selects the first mutually-supported encoding, while
// browsers typically advertise gzip before br. Gzip remains available when a
// client does not advertise Brotli.
func preferBrotli(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if acceptEncoding := brotliFirst(request.Header.Get("Accept-Encoding")); acceptEncoding != "" {
			request.Header.Set("Accept-Encoding", acceptEncoding)
		}
		next.ServeHTTP(writer, request)
	})
}

func brotliFirst(acceptEncoding string) string {
	encodings := strings.Split(acceptEncoding, ",")
	for index, encoding := range encodings {
		name, _, _ := strings.Cut(strings.TrimSpace(encoding), ";")
		if !strings.EqualFold(name, "br") || index == 0 {
			continue
		}
		encodings[0], encodings[index] = encodings[index], encodings[0]
		return strings.Join(encodings, ",")
	}
	return ""
}

// newUIHandler serves embedded static assets, negotiating Vite's build-time
// Brotli/Gzip variants via statigz. Requests that accept HTML are browser
// navigations and fall back to the SPA entrypoint; other missing resources
// return 404.
func newUIHandler(assets fs.ReadDirFS) http.Handler {
	fileServer := statigz.FileServer(assets, brotli.AddEncoding)

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if fileServer.Found(request) {
			fileServer.ServeHTTP(writer, request)
			return
		}
		if !acceptsHTML(request.Header.Get("Accept")) {
			http.NotFound(writer, request)
			return
		}
		// statigz serves index.html at "/" (http.FileServer would redirect
		// /index.html to /, so serve the entrypoint at the root directly).
		indexRequest := request.Clone(request.Context())
		indexRequest.URL.Path = "/"
		fileServer.ServeHTTP(writer, indexRequest)
	})
}

func acceptsHTML(accept string) bool {
	for value := range strings.SplitSeq(accept, ",") {
		mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(value))
		if err != nil || !strings.EqualFold(mediaType, "text/html") {
			continue
		}
		if quality, qualityErr := strconv.ParseFloat(params["q"], 64); qualityErr == nil && quality == 0 {
			continue
		}
		return true
	}
	return false
}

// embeddedUI returns the embedded frontend assets. UI builds place them under
// ui/dist; non-UI builds embed the placeholder index.html at the root.
func embeddedUI() fs.ReadDirFS {
	assets, err := fs.Sub(web.Dist, "ui/dist")
	if err != nil {
		return web.Dist
	}
	if _, err = fs.Stat(assets, "index.html"); err != nil {
		return web.Dist
	}
	embedded, ok := assets.(fs.ReadDirFS)
	if !ok {
		return web.Dist
	}
	return embedded
}

// Start begins listening for connections. The server runs in a background goroutine and is shut down when Stop is called.
func (s *Server) Start(ctx context.Context) error {
	var lc net.ListenConfig
	var err error
	s.listener, err = lc.Listen(ctx, "tcp", s.address)
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
