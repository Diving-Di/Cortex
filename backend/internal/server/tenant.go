package server

import (
	"net/http"

	"cortex/backend/internal/httpx"
)

type tenantUpdateRequest struct {
	Name string `json:"name"`
}

func (s *Server) getTenant(w http.ResponseWriter, r *http.Request) {
	value, err := s.tenants.Get(r.Context(), principalFrom(r.Context()))
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
	value, err := s.tenants.Update(r.Context(), principalFrom(r.Context()), request.Name)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, value)
}

func (s *Server) deleteTenant(w http.ResponseWriter, r *http.Request) {
	err := s.tenants.Delete(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
