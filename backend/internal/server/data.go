package server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"mime"
	"net/http"
	"regexp"
	"strings"

	"cortex/backend/internal/httpx"
)

var unsafeFilename = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

func (s *Server) exportMarkdown(w http.ResponseWriter, r *http.Request) {
	notes, err := s.store.ExportNotes(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	used := make(map[string]bool)
	for _, note := range notes {
		base := strings.Trim(unsafeFilename.ReplaceAllString(note.Title, "_"), " .")
		if base == "" {
			base = fmt.Sprintf("note-%d", note.ID)
		}
		date := "undated"
		if note.NoteDate != nil {
			date = *note.NoteDate
		}
		original := date + "-" + base + ".md"
		candidate := original
		for index := 2; used[strings.ToLower(candidate)]; index++ {
			candidate = strings.TrimSuffix(original, ".md") + fmt.Sprintf("-%d.md", index)
		}
		used[strings.ToLower(candidate)] = true
		writer, err := archive.Create(candidate)
		if err != nil {
			httpx.WriteError(w, s.logger, err)
			return
		}
		if _, err := writer.Write([]byte("# " + note.Title + "\n\n" + note.Content)); err != nil {
			httpx.WriteError(w, s.logger, err)
			return
		}
	}
	if err := archive.Close(); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	writeZip(w, "cortex-markdown.zip", output.Bytes())
}

func writeZip(w http.ResponseWriter, filename string, content []byte) {
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}
