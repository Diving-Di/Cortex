package server

import (
	"net/http"
	"strconv"
	"strings"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/httpx"
	"cortex/backend/internal/store"
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
	if err == nil && scope != "" && !store.ValidSourceScope(scope) {
		err = apierror.Validation(nil)
	}
	var result []store.Conversation
	var total int
	if err == nil {
		result, total, err = s.store.ListScopedConversations(r.Context(), principalFrom(r.Context()), strings.TrimSpace(r.URL.Query().Get("search")), scope, limit, offset)
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if result == nil {
		result = []store.Conversation{}
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
	request.Title = strings.TrimSpace(request.Title)
	if err == nil && (len([]rune(request.Title)) < 1 || len([]rune(request.Title)) > 255 || request.Version < 1) {
		err = apierror.Validation(nil)
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	result, err := s.store.RenameConversation(r.Context(), principalFrom(r.Context()), id, request.Title, request.Version)
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
	request.Title = strings.TrimSpace(request.Title)
	request.SourceScope = strings.TrimSpace(request.SourceScope)
	if request.Title == "" {
		request.Title = "新对话"
	}
	if len([]rune(request.Title)) > 80 || !store.ValidSourceScope(request.SourceScope) {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	result, err := s.store.CreateConversation(r.Context(), principalFrom(r.Context()), request.Title, request.SourceScope)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, result)
}

func (s *Server) getV1Conversation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "conversationID")
	if err == nil {
		var result store.Conversation
		result, err = s.store.GetConversation(r.Context(), principalFrom(r.Context()), id)
		if err == nil && !store.ValidSourceScope(result.SourceScope) {
			err = apierror.New("CONVERSATION_NOT_FOUND", "对话不存在", 404)
		}
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
		var result store.Conversation
		result, err = s.store.GetConversation(r.Context(), principalFrom(r.Context()), id)
		if err == nil && !store.ValidSourceScope(result.SourceScope) {
			err = apierror.New("CONVERSATION_NOT_FOUND", "对话不存在", 404)
		}
		if err == nil {
			err = s.store.DeleteConversation(r.Context(), principalFrom(r.Context()), id)
		}
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
