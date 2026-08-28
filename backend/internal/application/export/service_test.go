package export

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"testing"

	"cortex/backend/internal/domain"
)

type exportRepositoryStub struct{ Repository }

func (exportRepositoryStub) ExportNotes(context.Context, domain.Principal) ([]domain.ExportNote, error) {
	date := "2026-08-28"
	return []domain.ExportNote{{ID: 1, Title: "周报", Content: "正文", NoteDate: &date}}, nil
}

func TestMarkdownZIP(t *testing.T) {
	data, err := NewService(exportRepositoryStub{}).MarkdownZIP(context.Background(), domain.Principal{})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(archive.File) != 1 || archive.File[0].Name != "2026-08-28-周报.md" {
		t.Fatalf("archive=%v err=%v", archive.File, err)
	}
	reader, _ := archive.File[0].Open()
	content, _ := io.ReadAll(reader)
	_ = reader.Close()
	if string(content) != "# 周报\n\n正文" {
		t.Fatalf("content=%q", content)
	}
}
