//go:build ui

package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	httpserver "github.com/varunbpatil/temporal-lens/inbound/http"
)

func TestNewHandlerServesEmbeddedUIAsset(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	httpserver.NewHandler(http.NewServeMux()).ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/favicon.svg", nil),
	)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Header().Get("Content-Type"), "image/svg+xml")
}
