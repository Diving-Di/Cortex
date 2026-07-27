package server

import "testing"

func TestValidateAttachment(t *testing.T) {
	if _, mimeType, err := validateAttachment("note.md", []byte("中文")); err != nil || mimeType != "text/markdown" {
		t.Fatalf("valid markdown rejected: %v", err)
	}
	if _, _, err := validateAttachment("fake.png", []byte("not-png")); err == nil {
		t.Fatal("forged PNG accepted")
	}
	if _, _, err := validateAttachment("empty.txt", nil); err == nil {
		t.Fatal("empty file accepted")
	}
}
