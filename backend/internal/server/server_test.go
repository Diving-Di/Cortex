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

func TestLegacyRoutesAreNotRegistered(t *testing.T) {
	handler := New(
		config.Config{CORSOrigins: []string{"http://localhost:5173"}},
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		"test",
	)
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/register/"},
		{http.MethodPost, "/api/login/"},
		{http.MethodPost, "/api/logout/"},
		{http.MethodPost, "/api/chat/"},
		{http.MethodGet, "/api/chat/conversations/"},
		{http.MethodGet, "/api/chat/conversations/1/"},
		{http.MethodDelete, "/api/chat/conversations/1/"},
		{http.MethodGet, "/api/diary/"},
		{http.MethodPost, "/api/diary/"},
		{http.MethodDelete, "/api/diary/1/"},
		{http.MethodGet, "/api/dashboard"},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			request := httptest.NewRequest(route.method, route.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
			}
		})
	}
}

func TestRemovedRestoreAndBackupRoutesAreNotRegistered(t *testing.T) {
	handler := New(
		config.Config{CORSOrigins: []string{"http://localhost:5173"}},
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		"test",
	)
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/tenant/restore"},
		{http.MethodGet, "/api/v1/backups/full"},
		{http.MethodPost, "/api/v1/backups/full/restore"},
	}
	for _, route := range routes {
		request := httptest.NewRequest(route.method, route.path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s %s status = %d, want %d", route.method, route.path, response.Code, http.StatusNotFound)
		}
	}
}
