package server

import (
	"mime"
	"net/http"

	"cortex/backend/internal/httpx"
)

func (s *Server) exportMarkdown(w http.ResponseWriter, r *http.Request) {
	content, err := s.export.MarkdownZIP(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	writeZip(w, "cortex-markdown.zip", content)
}

func writeZip(w http.ResponseWriter, filename string, content []byte) {
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}
