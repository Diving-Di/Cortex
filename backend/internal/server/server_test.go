package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cortex/backend/internal/config"
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

func TestMetricsExposeLowCardinalityHTTPServiceIndicators(t *testing.T) {
	handler := New(
		config.Config{CORSOrigins: []string{"http://localhost:5173"}},
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		"test",
	)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/missing/123", nil))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := response.Body.String()
	for _, expected := range []string{
		`cortex_http_requests_total{method="GET",route="/healthz"}`,
		`cortex_http_requests_total{method="GET",route="unmatched"}`,
		`cortex_http_request_duration_seconds_bucket{method="GET",route="/healthz",le="+Inf"}`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics missing %q", expected)
		}
	}
}

func TestReadyOnlyDependsOnDatabase(t *testing.T) {
	handler := New(
		config.Config{CORSOrigins: []string{"http://localhost:5173"}},
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		"test",
	)
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready status without database = %d", response.Code)
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
