package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
	"cortex/backend/internal/httpx"
	"cortex/backend/internal/store"
)

type noteRequest struct {
	Type     string  `json:"type"`
	Title    string  `json:"title"`
	Content  string  `json:"content"`
	NoteDate *string `json:"note_date"`
	Summary  *string `json:"summary"`
}

func (s *Server) listNotes(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	page, err := positiveQueryInt(query.Get("page"), 1, 0)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	pageSize, err := positiveQueryInt(query.Get("page_size"), 20, 100)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	filter := store.NoteFilter{Page: page, PageSize: pageSize, Type: query.Get("type")}
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
		if parseErr != nil {
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
	items, total, err := s.store.ListNotes(r.Context(), principalFrom(r.Context()), filter)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	responses := make([]domain.NoteResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, item.Response())
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"items": responses, "page": page, "page_size": pageSize, "total": total,
	})
}

func (s *Server) createNote(w http.ResponseWriter, r *http.Request) {
	var request noteRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if request.Type == "" {
		request.Type = "normal"
	}
	if !validNoteType(request.Type) {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	title := strings.TrimSpace(request.Title)
	if title == "" {
		httpx.WriteError(w, s.logger, apierror.New("TITLE_REQUIRED", "标题不能为空", 422))
		return
	}
	noteDate, err := optionalDate(request.NoteDate)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if request.Type != "normal" && noteDate == nil {
		httpx.WriteError(w, s.logger, apierror.New("NOTE_DATE_REQUIRED", "周期笔记必须指定归属日期", 422))
		return
	}
	noteDate = normalizePeriod(request.Type, noteDate)
	note, err := s.store.CreateNote(r.Context(), principalFrom(r.Context()), store.NoteInput{
		Type: request.Type, Title: title, Content: request.Content,
		NoteDate: noteDate, Summary: request.Summary,
	})
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, note.Response())
}

func (s *Server) getNote(w http.ResponseWriter, r *http.Request) {
	noteID, err := pathID(r, "noteID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	note, err := s.store.GetNote(r.Context(), principalFrom(r.Context()), noteID)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, note.Response())
}

func (s *Server) updateNote(w http.ResponseWriter, r *http.Request) {
	noteID, err := pathID(r, "noteID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	var raw map[string]json.RawMessage
	if decodeErr := httpx.DecodeJSON(r, &raw); decodeErr != nil {
		httpx.WriteError(w, s.logger, decodeErr)
		return
	}
	allowed := map[string]bool{"title": true, "content": true, "note_date": true, "summary": true, "expected_updated_at": true}
	for key := range raw {
		if !allowed[key] {
			httpx.WriteError(w, s.logger, apierror.Validation(nil))
			return
		}
	}
	patch, parseErr := parsePatch(raw)
	if parseErr != nil {
		httpx.WriteError(w, s.logger, parseErr)
		return
	}
	note, err := s.store.UpdateNote(r.Context(), principalFrom(r.Context()), noteID, patch)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, note.Response())
}

func (s *Server) deleteNote(w http.ResponseWriter, r *http.Request) {
	noteID, err := pathID(r, "noteID")
	if err == nil {
		err = s.store.DeleteNote(r.Context(), principalFrom(r.Context()), noteID)
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listRevisions(w http.ResponseWriter, r *http.Request) {
	noteID, err := pathID(r, "noteID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	revisions, err := s.store.ListRevisions(r.Context(), principalFrom(r.Context()), noteID)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	response := make([]map[string]any, 0, len(revisions))
	for _, revision := range revisions {
		response = append(response, revision.Response())
	}
	httpx.JSON(w, http.StatusOK, response)
}

func (s *Server) restoreRevision(w http.ResponseWriter, r *http.Request) {
	noteID, err := pathID(r, "noteID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	revisionID, err := pathID(r, "revisionID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	note, err := s.store.RestoreRevision(r.Context(), principalFrom(r.Context()), noteID, revisionID)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, note.Response())
}

func parsePatch(raw map[string]json.RawMessage) (store.NotePatch, error) {
	var patch store.NotePatch
	if value, ok := raw["title"]; ok {
		var parsed string
		if err := json.Unmarshal(value, &parsed); err != nil {
			return patch, apierror.Validation(nil)
		}
		patch.Title = &parsed
	}
	if value, ok := raw["content"]; ok {
		var parsed string
		if err := json.Unmarshal(value, &parsed); err != nil {
			return patch, apierror.Validation(nil)
		}
		patch.Content = &parsed
	}
	if value, ok := raw["note_date"]; ok {
		patch.SetNoteDate = true
		if !bytes.Equal(value, []byte("null")) {
			var parsed string
			if err := json.Unmarshal(value, &parsed); err != nil {
				return patch, apierror.Validation(nil)
			}
			date, err := parseDate(parsed)
			if err != nil {
				return patch, err
			}
			patch.NoteDate = date
		}
	}
	if value, ok := raw["summary"]; ok {
		patch.SetSummary = true
		if !bytes.Equal(value, []byte("null")) {
			var parsed string
			if err := json.Unmarshal(value, &parsed); err != nil {
				return patch, apierror.Validation(nil)
			}
			patch.Summary = &parsed
		}
	}
	if value, ok := raw["expected_updated_at"]; ok && !bytes.Equal(value, []byte("null")) {
		var parsed string
		if err := json.Unmarshal(value, &parsed); err != nil {
			return patch, apierror.Validation(nil)
		}
		timestamp, err := time.Parse(time.RFC3339Nano, parsed)
		if err != nil {
			return patch, apierror.Validation(nil)
		}
		patch.ExpectedUpdatedAt = &timestamp
	}
	return patch, nil
}

func pathID(r *http.Request, name string) (int32, error) {
	value, err := strconv.ParseInt(r.PathValue(name), 10, 32)
	if err != nil || value <= 0 {
		return 0, apierror.Validation(nil)
	}
	return int32(value), nil
}

func positiveQueryInt(raw string, fallback, max int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || (max > 0 && value > max) {
		return 0, apierror.Validation(nil)
	}
	return value, nil
}

func parseDate(value string) (*time.Time, error) {
	date, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return nil, apierror.Validation(nil)
	}
	return &date, nil
}

func optionalDate(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	return parseDate(*value)
}

func normalizePeriod(noteType string, value *time.Time) *time.Time {
	if value == nil || noteType == "normal" || noteType == "daily" {
		return value
	}
	normalized := *value
	if noteType == "weekly" {
		days := (int(normalized.Weekday()) + 6) % 7
		normalized = normalized.AddDate(0, 0, -days)
	} else {
		normalized = time.Date(normalized.Year(), normalized.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	return &normalized
}

func validNoteType(value string) bool {
	return value == "normal" || value == "daily" || value == "weekly" || value == "monthly"
}
