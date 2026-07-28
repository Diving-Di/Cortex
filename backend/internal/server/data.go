package server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"mime"
	"net/http"
	"regexp"
	"strings"

	"diary-listener/backend/internal/httpx"
)

var unsafeFilename = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

func (s *Server) exportMarkdown(w http.ResponseWriter, r *http.Request) {
	notes, err := s.store.ExportNotes(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	memories, err := s.store.ExportGrowthMemories(r.Context(), principalFrom(r.Context()))
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
	if len(memories) > 0 {
		writer, err := archive.Create("growth-memories.md")
		if err != nil {
			httpx.WriteError(w, s.logger, err)
			return
		}
		var content strings.Builder
		content.WriteString("# 成长记忆\n\n")
		for _, memory := range memories {
			fmt.Fprintf(&content, "## %s · 重要度 %d\n\n%s\n\n- 来源类型：%s\n- 创建方式：%s\n\n",
				memory.Category, memory.Importance, memory.Content, memory.SourceType, memory.CreationMode)
		}
		if _, err := writer.Write([]byte(content.String())); err != nil {
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
