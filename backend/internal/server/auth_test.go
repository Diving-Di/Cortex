package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"diary-listener/backend/internal/config"
)

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
