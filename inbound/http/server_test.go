package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	httpserver "github.com/varunbpatil/temporal-lens/inbound/http"
)

func TestNewHandlerServesAPIAndUI(t *testing.T) {
	t.Parallel()

	api := http.NewServeMux()
	api.HandleFunc("/rpc", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	handler := httpserver.NewHandler(api)

	t.Run("API route", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/rpc", nil))

		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	for _, requestPath := range []string{"/", "/workflows/example"} {
		t.Run(requestPath, func(t *testing.T) {
			t.Parallel()

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, requestPath, nil))

			assert.Equal(t, http.StatusOK, response.Code)
			assert.Contains(t, response.Header().Get("Content-Type"), "text/html")
		})
	}

	t.Run("missing static asset", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))

		assert.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("unknown non-GET route", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/workflows/example", nil))

		assert.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("unknown API route", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/unknown", nil))

		assert.Equal(t, http.StatusNotFound, response.Code)
	})
}
