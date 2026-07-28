package knowledge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestParserClientBuildsDocumentFromPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Document-Format") != "txt" {
			t.Errorf("format header=%q", r.Header.Get("X-Document-Format"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pages":[{"page":1,"markdown":"# 标题\n\n正文"}],"page_count":1,"character_count":7}`))
	}))
	defer server.Close()
	file, err := os.CreateTemp(t.TempDir(), "source-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.WriteString("source"); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	document, err := (ParserClient{BaseURL: server.URL}).Parse(context.Background(), file.Name(), ".txt", "测试", ExtractLimits{MaxPages: 2, MaxCharacters: 100})
	if err != nil {
		t.Fatal(err)
	}
	if document.PageCount != 1 || len(document.Blocks) != 2 || document.Blocks[0].Kind != BlockHeading {
		t.Fatalf("unexpected document: %#v", document)
	}
}

func TestParserClientMapsOCRRequired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		_, _ = w.Write([]byte(`{"code":"DOCUMENT_OCR_REQUIRED"}`))
	}))
	defer server.Close()
	path := t.TempDir() + "/scan.pdf"
	if err := os.WriteFile(path, []byte("pdf"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := (ParserClient{BaseURL: server.URL}).Parse(context.Background(), path, ".pdf", "scan", ExtractLimits{MaxPages: 2, MaxCharacters: 100})
	if err != ErrOCRRequired {
		t.Fatalf("error=%v", err)
	}
}
