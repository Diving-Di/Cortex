package server

import (
	"net/http"
	"strconv"
	"strings"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/httpx"
	"cortex/backend/internal/store"
)

type tagCreateRequest struct {
	Name  string  `json:"name"`
	Color *string `json:"color"`
}

type tagAssignmentRequest struct {
	TagIDs []int32 `json:"tag_ids"`
}

func (s *Server) listTags(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListTags(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if items == nil {
		items = []store.Tag{}
	}
	httpx.JSON(w, http.StatusOK, items)
}

func (s *Server) createTag(w http.ResponseWriter, r *http.Request) {
	var request tagCreateRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" {
		httpx.WriteError(w, s.logger, apierror.New("TAG_NAME_REQUIRED", "标签名称不能为空", 422))
		return
	}
	item, err := s.store.CreateTag(r.Context(), principalFrom(r.Context()), request.Name, request.Color)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, item)
}

func (s *Server) listNoteTags(w http.ResponseWriter, r *http.Request) {
	noteID, err := pathID(r, "noteID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	items, err := s.store.ListNoteTags(r.Context(), principalFrom(r.Context()), noteID)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if items == nil {
		items = []store.Tag{}
	}
	httpx.JSON(w, http.StatusOK, items)
}

func (s *Server) assignNoteTags(w http.ResponseWriter, r *http.Request) {
	noteID, err := pathID(r, "noteID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	var request tagAssignmentRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	items, err := s.store.AssignNoteTags(r.Context(), principalFrom(r.Context()), noteID, request.TagIDs)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if items == nil {
		items = []store.Tag{}
	}
	httpx.JSON(w, http.StatusOK, items)
}

func (s *Server) searchNotes(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit, err := positiveQueryInt(query.Get("limit"), 20, 100)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	filter := store.SearchFilter{
		Query: query.Get("q"), Type: query.Get("type"), Limit: limit,
	}
	if filter.Type != "" && !validNoteType(filter.Type) {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	if value := query.Get("start_date"); value != "" {
		filter.StartDate, err = parseDate(value)
	}
	if err == nil {
		if value := query.Get("end_date"); value != "" {
			filter.EndDate, err = parseDate(value)
		}
	}
	if err == nil && query.Get("tag_id") != "" {
		value, parseErr := strconv.ParseInt(query.Get("tag_id"), 10, 32)
		if parseErr != nil || value <= 0 {
			err = apierror.Validation(nil)
		} else {
			tagID := int32(value)
			filter.TagID = &tagID
		}
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	items, total, err := s.store.SearchNotes(r.Context(), principalFrom(r.Context()), filter)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if items == nil {
		items = []store.SearchItem{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	timezoneName := r.URL.Query().Get("timezone")
	if timezoneName == "" {
		timezoneName = "Asia/Shanghai"
	}
	if len(timezoneName) > 64 {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	result, err := s.store.Dashboard(r.Context(), principalFrom(r.Context()), timezoneName)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}
