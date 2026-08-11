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
	var request loginRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	token, username, err := s.store.Login(
		r.Context(), strings.TrimSpace(request.Username), request.Password, s.cfg.TokenTTL,
	)
	if err != nil {
		if appErr, ok := err.(*apierror.Error); ok && appErr.Code == "INVALID_CREDENTIALS" {
			httpx.JSON(w, http.StatusBadRequest, map[string]string{"detail": appErr.Message})
			return
		}
		httpx.WriteError(w, s.logger, err)
		return
	}
	s.setAuthCookie(w, r, token, s.cfg.TokenTTL)
	httpx.JSON(w, http.StatusOK, map[string]string{"token": token, "username": username})
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r.Context())
	httpx.JSON(w, http.StatusOK, map[string]any{
		"username":      principal.Username,
		"tenant_active": principal.TenantActive,
	})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.store.RevokeToken(r.Context(), principalFrom(r.Context()).TokenID); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
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
