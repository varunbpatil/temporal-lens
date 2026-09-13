// Package observability provides router-neutral middleware for inbound servers.
package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
)

// RequestIDHeader is the HTTP header used to carry a correlation ID.
const RequestIDHeader = "X-Request-ID"

type requestIDContextKey struct{}

// RequestID assigns each request an ID, makes it available in the context, and
// echoes it in request and response headers.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID := strings.TrimSpace(request.Header.Get(RequestIDHeader))
		if requestID == "" {
			var err error
			requestID, err = newRequestID()
			if err != nil {
				http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		}
		request.Header.Set(RequestIDHeader, requestID)
		writer.Header().Set(RequestIDHeader, requestID)
		next.ServeHTTP(
			writer,
			request.WithContext(context.WithValue(request.Context(), requestIDContextKey{}, requestID)),
		)
	})
}

// Recoverer logs panics and sends a generic 500 response. It preserves
// [http.ErrAbortHandler], matching net/http's panic handling semantics.
func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			defer recoverPanic(logger, writer, request)
			next.ServeHTTP(writer, request)
		})
	}
}

func recoverPanic(logger *slog.Logger, writer http.ResponseWriter, request *http.Request) {
	recovered := recover()
	if recovered == nil {
		return
	}
	if err, ok := recovered.(error); ok && errors.Is(err, http.ErrAbortHandler) {
		panic(recovered)
	}
	logPanic(logger, request, recovered)
	http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func logPanic(logger *slog.Logger, request *http.Request, recovered any) {
	if logger == nil {
		return
	}
	attrs := []any{
		"method", request.Method,
		"path", request.URL.Path,
		"panic", recovered,
		"stack", string(debug.Stack()),
	}
	if requestID := RequestIDFromContext(request.Context()); requestID != "" {
		attrs = append(attrs, "request_id", requestID)
	}
	logger.ErrorContext(request.Context(), "HTTP handler panic", attrs...)
}

// RequestIDFromContext returns the request ID assigned by RequestID, if any.
func RequestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

func newRequestID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
