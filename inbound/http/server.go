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
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

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

// NewServer creates a new HTTP server that serves the API, MCP and embedded UI.
func NewServer(api, mcp http.Handler, address string, logger *slog.Logger, onFatal func(error)) *Server {
	return &Server{
		handler: NewHandler(api, mcp, logger),
		address: address,
		logger:  logger,
		onFatal: onFatal,
	}
}

// NewHandler serves Connect API routes below /api, MCP at /mcp, and the
// embedded UI at the root. Browser navigation falls back to the UI entrypoint;
// missing static assets return 404.
func NewHandler(api, mcp http.Handler, logger *slog.Logger) http.Handler {
	ui := newUIHandler(embeddedUI())
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(requestLogger(logger))
	router.Use(middleware.Recoverer)

	router.Handle("/mcp", mcp)
	router.Route(apiPrefix, func(router chi.Router) {
		router.Use(preferBrotli)
		router.Handle("/", http.NotFoundHandler())
		router.Handle("/*", http.StripPrefix(apiPrefix, api))
	})
	router.Get("/*", ui.ServeHTTP)
	router.Head("/*", ui.ServeHTTP)
	router.NotFound(http.NotFound)

	return router
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
// Brotli variants. Extensionless paths serve the SPA entrypoint.
func newUIHandler(assets fs.FS) http.Handler {
	files := http.FileServer(http.FS(assets))

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assetPath := strings.TrimPrefix(path.Clean(request.URL.Path), "/")
		info, err := fs.Stat(assets, assetPath)
		if err != nil || info.IsDir() {
			if path.Ext(request.URL.Path) != "" {
				http.NotFound(writer, request)
				return
			}
			assetPath = "index.html"
		}

		assetRequest := request.Clone(request.Context())
		assetRequest.URL.Path = "/" + assetPath
		// http.FileServer redirects /index.html to /. Serve the entrypoint at the
		// filesystem root so SPA fallbacks do not loop through that redirect.
		if assetPath == "index.html" {
			assetRequest.URL.Path = "/"
		}
		serveAsset(files, assets, writer, assetRequest, assetPath)
	})
}

func serveAsset(files http.Handler, assets fs.FS, writer http.ResponseWriter, request *http.Request, assetPath string) {
	variant, hasCompressedAsset := preferredCompression(request, assets, assetPath)
	if !hasCompressedAsset {
		files.ServeHTTP(writer, request)
		return
	}

	writer.Header().Add("Vary", "Accept-Encoding")
	if variant == nil {
		files.ServeHTTP(writer, request)
		return
	}

	compressedRequest := request.Clone(request.Context())
	compressedRequest.URL.Path = "/" + assetPath + variant.extension
	writer.Header().Set("Content-Encoding", variant.encoding)
	if contentType := mime.TypeByExtension(path.Ext(assetPath)); contentType != "" {
		writer.Header().Set("Content-Type", contentType)
	}
	files.ServeHTTP(writer, compressedRequest)
}

type compressionVariant struct {
	encoding  string
	extension string
}

func compressionVariants() [2]compressionVariant {
	return [2]compressionVariant{
		{encoding: "br", extension: ".br"},
		{encoding: "gzip", extension: ".gz"},
	}
}

// preferredCompression selects the supported encoding with the highest client
// preference. Brotli wins ties because it generally produces smaller assets.
// The second return value reports whether any pre-compressed variant exists,
// so callers can set Vary for the uncompressed fallback too.
func preferredCompression(request *http.Request, assets fs.FS, assetPath string) (*compressionVariant, bool) {
	var selected *compressionVariant
	selectedQuality := 0.0
	hasCompressedAsset := false
	variants := compressionVariants()
	for index := range variants {
		variant := &variants[index]
		if _, err := fs.Stat(assets, assetPath+variant.extension); err != nil {
			continue
		}
		hasCompressedAsset = true
		if quality := encodingQuality(request, variant.encoding); quality > selectedQuality {
			selected = variant
			selectedQuality = quality
		}
	}
	return selected, hasCompressedAsset
}

func encodingQuality(request *http.Request, wanted string) float64 {
	wildcardQuality := -1.0
	for encoding := range strings.SplitSeq(request.Header.Get("Accept-Encoding"), ",") {
		parts := strings.Split(encoding, ";")
		name := strings.TrimSpace(parts[0])
		if !strings.EqualFold(name, wanted) && name != "*" {
			continue
		}
		quality := 1.0
		for _, parameter := range parts[1:] {
			key, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if !ok || !strings.EqualFold(key, "q") {
				continue
			}
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				quality = 0
				break
			}
			quality = parsed
		}
		if strings.EqualFold(name, wanted) {
			return quality
		}
		wildcardQuality = quality
	}
	return max(wildcardQuality, 0)
}

// requestLogger uses Chi's request logging middleware with the application's
// structured logger. Static UI assets are omitted to keep page loads quiet.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return middleware.RequestLogger(slogLogFormatter{logger: logger})
}

type slogLogFormatter struct {
	logger *slog.Logger
}

func (formatter slogLogFormatter) NewLogEntry(request *http.Request) middleware.LogEntry {
	return slogLogEntry{
		logger:  formatter.logger,
		request: request,
		log:     strings.HasPrefix(request.URL.Path, apiPrefix) || request.URL.Path == "/mcp",
	}
}

type slogLogEntry struct {
	logger  *slog.Logger
	request *http.Request
	log     bool
}

func (entry slogLogEntry) Write(status, bytes int, _ http.Header, elapsed time.Duration, _ any) {
	if !entry.log {
		return
	}
	if status == 0 {
		status = http.StatusOK
	}

	attributes := []any{
		"request_id", middleware.GetReqID(entry.request.Context()),
		"method", entry.request.Method,
		"path", entry.request.URL.Path,
		"status", status,
		"bytes", bytes,
		"duration", elapsed,
	}
	if status >= http.StatusInternalServerError {
		entry.logger.ErrorContext(entry.request.Context(), "HTTP request failed", attributes...)
		return
	}
	entry.logger.InfoContext(entry.request.Context(), "HTTP request", attributes...)
}

func (entry slogLogEntry) Panic(value any, stack []byte) {
	entry.logger.ErrorContext(entry.request.Context(), "HTTP panic recovered",
		"request_id", middleware.GetReqID(entry.request.Context()),
		"method", entry.request.Method,
		"path", entry.request.URL.Path,
		"panic", value,
		"stack", string(stack),
	)
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
