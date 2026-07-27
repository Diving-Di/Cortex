package server

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"diary-listener/backend/internal/store"
)

func TestValidateKnowledgeTXT(t *testing.T) {
	path := filepath.Join(t.TempDir(), "valid.txt")
	if err := os.WriteFile(path, []byte("中英文 Cortex knowledge"), 0600); err != nil {
		t.Fatal(err)
	}
	mimeType, err := validateKnowledgeFile(path, ".txt", 1<<20)
	if err != nil || mimeType != "text/plain" {
		t.Fatalf("valid txt rejected: mime=%q err=%v", mimeType, err)
	}
	if err := os.WriteFile(path, []byte{0xff, 0x00}, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateKnowledgeFile(path, ".txt", 1<<20); err == nil {
		t.Fatal("binary txt accepted")
	}
}

func TestLimitKnowledgeContextEnforcesUnifiedBudget(t *testing.T) {
	sources := []store.KnowledgeCandidate{
		{Parent: strings.Repeat("证据", 30), Child: strings.Repeat("引用", 10)},
		{Parent: strings.Repeat("后续", 30), Child: strings.Repeat("片段", 10)},
	}
	result := limitKnowledgeContext(sources, 100)
	if len(result) != 1 {
		t.Fatalf("expected one source inside budget, got %d", len(result))
	}
}

func TestValidateKnowledgePDF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "valid.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateKnowledgeFile(path, ".pdf", 1<<20); err != nil {
		t.Fatalf("valid pdf signature rejected: %v", err)
	}
	if err := os.WriteFile(path, []byte("not pdf"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateKnowledgeFile(path, ".pdf", 1<<20); err == nil {
		t.Fatal("forged pdf accepted")
	}
}

func TestValidateKnowledgeDOCX(t *testing.T) {
	path := filepath.Join(t.TempDir(), "valid.docx")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	for _, name := range []string{"[Content_Types].xml", "word/document.xml"} {
		writer, createErr := archive.Create(name)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, writeErr := writer.Write([]byte("<xml/>")); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := validateKnowledgeFile(path, ".docx", 1<<20); err != nil {
		t.Fatalf("valid docx rejected: %v", err)
	}
}

func TestValidateKnowledgeDOCXRejectsZipSlip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "slip.docx")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	for _, name := range []string{"[Content_Types].xml", "word/document.xml", "../escape"} {
		writer, createErr := archive.Create(name)
		if createErr != nil {
			t.Fatal(createErr)
		}
		_, _ = writer.Write([]byte("<xml/>"))
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := validateKnowledgeFile(path, ".docx", 1<<20); err == nil {
		t.Fatal("Zip Slip DOCX accepted")
	}
}
