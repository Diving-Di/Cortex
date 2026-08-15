package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cortex/backend/internal/config"
)

func TestBrowserLoginResponseDoesNotExposeToken(t *testing.T) {
	response := httptest.NewRecorder()
	writeBrowserLoginResponse(response, "tester")
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["username"] != "tester" {
		t.Fatalf("username = %#v", body["username"])
	}
	if _, exposed := body["token"]; exposed {
		t.Fatal("browser login response exposed token")
	}
}

func TestSessionResponseDoesNotExposeTenantState(t *testing.T) {
	response := httptest.NewRecorder()
	writeSessionResponse(response, "tester")
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, exposed := body["tenant_active"]; exposed {
		t.Fatal("session response exposed tenant state")
	}
}

func TestTokenEndpointRejectsBrowserOrigin(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token", nil)
	request.Header.Set("Origin", "https://cortex.example")
	response := httptest.NewRecorder()
	server.issueToken(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestAuthCookieIsHttpOnlyAndStrict(t *testing.T) {
	server := &Server{cfg: config.Config{Environment: "development"}}
	request := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	response := httptest.NewRecorder()

	server.setAuthCookie(response, request, "secret", time.Hour)
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookie count = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != authCookieName || cookie.Value != "secret" {
		t.Fatalf("unexpected auth cookie: %#v", cookie)
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/api/v1" {
		t.Fatalf("auth cookie is missing security attributes: %#v", cookie)
	}
}

func TestClearAuthCookieExpiresIt(t *testing.T) {
	server := &Server{cfg: config.Config{Environment: "development"}}
	request := httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
	response := httptest.NewRecorder()

	server.clearAuthCookie(response, request)
	cookie := response.Result().Cookies()[0]
	if cookie.MaxAge >= 0 || cookie.Value != "" {
		t.Fatalf("auth cookie was not expired: %#v", cookie)
	}
}
