package server

import (
    "net/http"
    "strings"

    "diary-listener/backend/internal/apierror"
    "diary-listener/backend/internal/auth"
    "diary-listener/backend/internal/httpx"
)

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
    httpx.JSON(w, http.StatusOK, map[string]string{"token": token, "username": username})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
    if err := s.store.RevokeToken(r.Context(), principalFrom(r.Context()).TokenID); err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    w.WriteHeader(http.StatusNoContent)
}
