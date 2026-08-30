package server

import (
	"net/http"
	"strings"
	"time"

	"cortex/backend/internal/apierror"
	authapp "cortex/backend/internal/application/auth"
	"cortex/backend/internal/httpx"
)

const authCookieName = "cortex_session"

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type browserLoginResponse struct {
	Username string `json:"username"`
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var request registerRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if !s.allowIdentityRequest(r, "auth-register-user", request.Username, 3, time.Hour) ||
		!s.allowIdentityRequest(r, "auth-register-email", request.Email, 3, time.Hour) {
		httpx.WriteError(w, s.logger, apierror.New("RATE_LIMITED", "注册请求过于频繁，请稍后重试", http.StatusTooManyRequests))
		return
	}
	if err := s.authService.Register(r.Context(), request.Username, request.Email, request.Password); err != nil {
		if validationErr, ok := err.(*authapp.ValidationError); ok {
			httpx.WriteError(w, s.logger, apierror.New("VALIDATION_ERROR", validationErr.Message, http.StatusBadRequest))
			return
		}
		if appErr, ok := err.(*apierror.Error); ok && appErr.Code == "REGISTRATION_CONFLICT" {
			httpx.WriteError(w, s.logger, appErr)
			return
		}
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]string{"status": "registered"})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	token, username, ok := s.authenticateCredentials(w, r, "auth-login")
	if !ok {
		return
	}
	s.setAuthCookie(w, r, token, s.cfg.TokenTTL)
	writeBrowserLoginResponse(w, username)
}

func writeBrowserLoginResponse(w http.ResponseWriter, username string) {
	httpx.JSON(w, http.StatusOK, browserLoginResponse{Username: username})
}

func (s *Server) issueToken(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("Origin")) != "" {
		httpx.WriteError(w, s.logger, apierror.New(
			"TOKEN_BROWSER_FORBIDDEN", "浏览器客户端必须使用会话 Cookie", http.StatusForbidden,
		))
		return
	}
	token, username, ok := s.authenticateCredentials(w, r, "auth-token")
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"token": token, "username": username})
}

func (s *Server) authenticateCredentials(w http.ResponseWriter, r *http.Request, scope string) (string, string, bool) {
	var request loginRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, s.logger, err)
		return "", "", false
	}
	limit := s.cfg.AuthLoginAccountLimit
	if limit <= 0 {
		limit = 10
	}
	if !s.allowIdentityRequest(r, scope, request.Username, limit, 5*time.Minute) {
		httpx.WriteError(w, s.logger, apierror.New("RATE_LIMITED", "登录请求过于频繁，请稍后重试", http.StatusTooManyRequests))
		return "", "", false
	}
	token, username, err := s.authService.Login(
		r.Context(), strings.TrimSpace(request.Username), request.Password, s.cfg.TokenTTL,
	)
	if err != nil {
		if appErr, ok := err.(*apierror.Error); ok && appErr.Code == "INVALID_CREDENTIALS" {
			httpx.WriteError(w, s.logger, appErr)
			return "", "", false
		}
		httpx.WriteError(w, s.logger, err)
		return "", "", false
	}
	// Populate the digest-keyed cache before returning the raw token so the
	// client's first authenticated request does not need another database read.
	if _, err := s.resolvePrincipal(r.Context(), token); err != nil {
		httpx.WriteError(w, s.logger, err)
		return "", "", false
	}
	return token, username, true
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r.Context())
	writeSessionResponse(w, principal.Username)
}

func writeSessionResponse(w http.ResponseWriter, username string) {
	httpx.JSON(w, http.StatusOK, map[string]string{"username": username})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r.Context())
	if err := s.authService.Revoke(r.Context(), p.TokenID); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if s.authRedis != nil && p.AuthCacheKey != "" {
		_ = s.authRedis.Delete(r.Context(), p.AuthCacheKey)
	}
	s.clearAuthCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setAuthCookie(w http.ResponseWriter, r *http.Request, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: authCookieName, Value: token, Path: "/api/v1", HttpOnly: true,
		Secure:   s.cfg.Environment == "production" || r.TLS != nil,
		SameSite: http.SameSiteStrictMode, MaxAge: int(ttl.Seconds()),
		Expires: time.Now().UTC().Add(ttl),
	})
}

func (s *Server) clearAuthCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: authCookieName, Value: "", Path: "/api/v1", HttpOnly: true,
		Secure:   s.cfg.Environment == "production" || r.TLS != nil,
		SameSite: http.SameSiteStrictMode, MaxAge: -1, Expires: time.Unix(1, 0),
	})
}
