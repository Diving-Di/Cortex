package server

import (
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"cortex/backend/internal/apierror"
	attachmentsapp "cortex/backend/internal/application/attachments"
	"cortex/backend/internal/domain"
	"cortex/backend/internal/httpx"
)

func (s *Server) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	noteIDRaw := r.URL.Query().Get("note_id")
	noteID, err := parsePositiveInt32(noteIDRaw)
	if err != nil {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxAttachmentBytes+(1<<20))
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, s.cfg.MaxAttachmentBytes+1))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if int64(len(data)) > s.cfg.MaxAttachmentBytes {
		httpx.WriteError(w, s.logger, apierror.New("ATTACHMENT_TOO_LARGE", "附件超过单文件大小限制", 413))
		return
	}
	principal := principalFrom(r.Context())
	item, err := s.attachments.Upload(r.Context(), principal, noteID, header.Filename, data)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, item.Response())
}

func (s *Server) listAttachments(w http.ResponseWriter, r *http.Request) {
	noteID, err := pathID(r, "noteID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	items, err := s.attachments.List(r.Context(), principalFrom(r.Context()), noteID)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	result := make([]domain.AttachmentResponse, 0, len(items))
	for _, item := range items {
		result = append(result, item.Response())
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (s *Server) downloadAttachment(w http.ResponseWriter, r *http.Request) {
	attachmentID, err := pathID(r, "attachmentID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	download, err := s.attachments.Download(r.Context(), principalFrom(r.Context()), attachmentID)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	defer download.Body.Close()
	w.Header().Set("Content-Type", download.Item.MIMEType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": download.Item.OriginalName}))
	w.Header().Set("Content-Length", strconv.FormatInt(download.Size, 10))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, download.Body)
}

func (s *Server) deleteAttachment(w http.ResponseWriter, r *http.Request) {
	attachmentID, err := pathID(r, "attachmentID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	principal := principalFrom(r.Context())
	if err := s.attachments.Delete(r.Context(), principal, attachmentID); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) safeDataPath(relative, expectedRoot string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(relative))
	if filepath.IsAbs(cleaned) || cleaned == "." || cleaned == ".." ||
		!strings.HasPrefix(cleaned, expectedRoot+string(filepath.Separator)) {
		return "", apierror.New("INVALID_ATTACHMENT_PATH", "附件路径无效", 500)
	}
	root := filepath.Join(s.cfg.DataDir, expectedRoot)
	target := filepath.Join(s.cfg.DataDir, cleaned)
	relativeToRoot, err := filepath.Rel(root, target)
	if err != nil || relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, ".."+string(filepath.Separator)) {
		return "", apierror.New("INVALID_ATTACHMENT_PATH", "附件路径无效", 500)
	}
	return target, nil
}

// Compatibility wrappers keep existing package-level callers while the
// attachment use case and its validation live in the application layer.
func validateAttachment(filename string, data []byte) (string, string, error) {
	return attachmentsapp.Validate(filename, data)
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func parsePositiveInt32(raw string) (int32, error) {
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || value <= 0 {
		return 0, apierror.Validation(nil)
	}
	return int32(value), nil
}
