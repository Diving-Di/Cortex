package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"diary-listener/backend/internal/config"
)

func TestRouterStartsAndServesHealth(t *testing.T) {
	handler := New(
		config.Config{CORSOrigins: []string{"http://localhost:5173"}},
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		"test",
	)
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("health status = %d", response.Code)
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing response request ID")
	}
}
