//nolint:testpackage // The HTTP logging middleware is intentionally internal to the adapter.
package http

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoggingHandlerLogsAPIErrors(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	handler := newLoggingHandler(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "failed", http.StatusInternalServerError)
	}), logger)

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/workflows", nil))

	require.Contains(t, logs.String(), "msg=\"HTTP request failed\"")
	require.Contains(t, logs.String(), "status=500")
	require.Contains(t, logs.String(), "path=/api/workflows")
}
