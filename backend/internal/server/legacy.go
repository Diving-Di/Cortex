package server

import (
	"net/http"
	"strconv"
	"strings"

	"cortex/backend/internal/domain"
	"cortex/backend/internal/httpx"
)

type conversationRequest struct {
	Title       string `json:"title"`
	SourceScope string `json:"source_scope"`
}

func (s *Server) listV1Conversations(w http.ResponseWriter, r *http.Request) {
	limit, err := positiveQueryInt(r.URL.Query().Get("limit"), 20, 100)
	offset := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		offset, _ = strconv.Atoi(raw)
	}
	scope := strings.TrimSpace(r.URL.Query().Get("source_scope"))
	var result []domain.Conversation
	var total int
	if err == nil {
		result, total, err = s.conversations.List(r.Context(), principalFrom(r.Context()), r.URL.Query().Get("search"), scope, limit, offset)
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if result == nil {
		result = []domain.Conversation{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": result, "total": total})
}

func (s *Server) renameV1Conversation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "conversationID")
	var request struct {
		Title   string `json:"title"`
		Version int    `json:"version"`
	}
	if err == nil {
		if decodeErr := httpx.DecodeJSON(r, &request); decodeErr != nil {
			err = decodeErr
		}
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	result, err := s.conversations.Rename(r.Context(), principalFrom(r.Context()), id, request.Title, request.Version)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (s *Server) createV1Conversation(w http.ResponseWriter, r *http.Request) {
	var request conversationRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	result, err := s.conversations.Create(r.Context(), principalFrom(r.Context()), request.Title, request.SourceScope)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, result)
}

func (s *Server) getV1Conversation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "conversationID")
	if err == nil {
		var result domain.Conversation
		result, err = s.conversations.Get(r.Context(), principalFrom(r.Context()), id)
		if err == nil {
			httpx.JSON(w, http.StatusOK, result)
			return
		}
	}
	httpx.WriteError(w, s.logger, err)
}

func (s *Server) deleteV1Conversation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "conversationID")
	if err == nil {
		err = s.conversations.Delete(r.Context(), principalFrom(r.Context()), id)
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
