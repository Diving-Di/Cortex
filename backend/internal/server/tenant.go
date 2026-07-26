package server

import (
    "net/http"
    "strings"

    "diary-listener/backend/internal/apierror"
    "diary-listener/backend/internal/httpx"
)

type tenantUpdateRequest struct {
    Name string `json:"name"`
}

func (s *Server) getTenant(w http.ResponseWriter, r *http.Request) {
    value, err := s.store.GetTenant(r.Context(), principalFrom(r.Context()))
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    httpx.JSON(w, http.StatusOK, value)
}

func (s *Server) updateTenant(w http.ResponseWriter, r *http.Request) {
    var request tenantUpdateRequest
    if err := httpx.DecodeJSON(r, &request); err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    request.Name = strings.TrimSpace(request.Name)
    if request.Name == "" {
        httpx.WriteError(w, s.logger, apierror.New("TENANT_NAME_REQUIRED", "个人空间名称不能为空", 422))
        return
    }
    value, err := s.store.UpdateTenant(r.Context(), principalFrom(r.Context()), request.Name)
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    httpx.JSON(w, http.StatusOK, value)
}

func (s *Server) deleteTenant(w http.ResponseWriter, r *http.Request) {
    if err := s.store.DeleteTenant(r.Context(), principalFrom(r.Context())); err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    w.WriteHeader(http.StatusNoContent)
}

func (s *Server) restoreTenant(w http.ResponseWriter, r *http.Request) {
    value, err := s.store.RestoreTenant(r.Context(), principalFrom(r.Context()))
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    httpx.JSON(w, http.StatusOK, value)
}
