package documentparser

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseContractAndMarkdown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/parse" || r.Header.Get("X-Filename") != "report.pdf" {
			t.Fatalf("request=%s filename=%q", r.URL.Path, r.Header.Get("X-Filename"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"blocks":[{"block_type":"paragraph","text":"正文","page":2,"parser_version":"test"}]}`))
	}))
	defer server.Close()
	result, err := (Client{BaseURL: server.URL, Timeout: time.Second, MaxBody: 1024}).Parse(context.Background(), "folder/report.pdf", []byte("%PDF-"))
	if err != nil {
		t.Fatal(err)
	}
	if got := Markdown(result); got != "## 第 2 页\n正文" {
		t.Fatalf("markdown=%q", got)
	}
}

func TestParseMapsStableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(422) }))
	defer server.Close()
	_, err := (Client{BaseURL: server.URL, Timeout: time.Second}).Parse(context.Background(), "bad.pdf", []byte("x"))
	var parserErr *Error
	if !errors.As(err, &parserErr) || parserErr.Code != "KNOWLEDGE_DOCUMENT_UNSAFE" {
		t.Fatalf("error=%v", err)
	}
}

func TestParseRejectsEmptyBlocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"blocks":[]}`)) }))
	defer server.Close()
	_, err := (Client{BaseURL: server.URL, Timeout: time.Second}).Parse(context.Background(), "empty.pdf", []byte("x"))
	var parserErr *Error
	if !errors.As(err, &parserErr) || parserErr.Code != "KNOWLEDGE_DOCUMENT_EMPTY" {
		t.Fatalf("error=%v", err)
	}
}
