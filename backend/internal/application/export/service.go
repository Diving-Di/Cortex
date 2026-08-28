package export

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"

	"cortex/backend/internal/domain"
)

var unsafeFilename = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

type Repository interface {
	ExportNotes(context.Context, domain.Principal) ([]domain.ExportNote, error)
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) MarkdownZIP(ctx context.Context, p domain.Principal) ([]byte, error) {
	notes, err := s.repository.ExportNotes(ctx, p)
	if err != nil {
		return nil, err
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
		writer, createErr := archive.Create(candidate)
		if createErr != nil {
			return nil, createErr
		}
		if _, writeErr := writer.Write([]byte("# " + note.Title + "\n\n" + note.Content)); writeErr != nil {
			return nil, writeErr
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
