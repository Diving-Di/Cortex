package server

import (
	"net/http"
	"strings"
	"time"

	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/auth"
	"diary-listener/backend/internal/httpx"
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
	request.Username = strings.TrimSpace(request.Username)
	request.Email = strings.TrimSpace(request.Email)
	if len(request.Username) < 6 {
		httpx.JSON(w, http.StatusBadRequest, map[string]string{"detail": "用户名长度需至少 6 个字符"})
		return
	}
	if len(request.Password) < 6 {
		httpx.JSON(w, http.StatusBadRequest, map[string]string{"detail": "密码长度需至少 6 个字符"})
		return
	}
	passwordHash, err := auth.HashPassword(request.Password)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if err := s.store.Register(r.Context(), request.Username, request.Email, passwordHash); err != nil {
		if appErr, ok := err.(*apierror.Error); ok && appErr.Code == "REGISTRATION_CONFLICT" {
			httpx.JSON(w, http.StatusBadRequest, map[string]string{"detail": appErr.Message})
			return
		}
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]string{"detail": "registered"})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	token, username, ok := s.authenticateCredentials(w, r)
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
	token, username, ok := s.authenticateCredentials(w, r)
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"token": token, "username": username})
}

func (s *Server) authenticateCredentials(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	var request loginRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, s.logger, err)
		return "", "", false
	}
	token, username, err := s.store.Login(
		r.Context(), strings.TrimSpace(request.Username), request.Password, s.cfg.TokenTTL,
	)
	if err != nil {
		if appErr, ok := err.(*apierror.Error); ok && appErr.Code == "INVALID_CREDENTIALS" {
			httpx.JSON(w, http.StatusBadRequest, map[string]string{"detail": appErr.Message})
			return "", "", false
		}
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
	if err := s.store.RevokeToken(r.Context(), p.TokenID); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if s.redis != nil && p.AuthCacheKey != "" {
		_ = s.redis.Delete(r.Context(), p.AuthCacheKey)
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
