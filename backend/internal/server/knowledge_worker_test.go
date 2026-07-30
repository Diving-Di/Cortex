package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"diary-listener/backend/internal/config"
	"diary-listener/backend/internal/knowledge"
	"diary-listener/backend/internal/store"
)

func TestExtractKnowledgeDocumentUsesLocalParserForMarkdown(t *testing.T) {
	var parserCalls atomic.Int32
	parser := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		parserCalls.Add(1)
		http.Error(w, `{"code":"DOCUMENT_UNSUPPORTED_TYPE"}`, http.StatusUnprocessableEntity)
	}))
	defer parser.Close()

	path := filepath.Join(t.TempDir(), "sample.md")
	if err := os.WriteFile(path, []byte("# 标题\n\nMarkdown 正文"), 0600); err != nil {
		t.Fatal(err)
	}
	document, err := extractKnowledgeDocument(
		context.Background(),
		config.Config{DocumentParserURL: parser.URL},
		path,
		store.KnowledgeDocument{Extension: ".md", OriginalName: "sample.md"},
		knowledge.ExtractLimits{MaxPages: 10, MaxCharacters: 1000, TimeoutSecs: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	if parserCalls.Load() != 0 {
		t.Fatal("Markdown was sent to the external document parser")
	}
	if len(document.Blocks) != 2 || document.Blocks[0].Kind != knowledge.BlockHeading {
		t.Fatalf("unexpected Markdown extraction: %#v", document.Blocks)
	}
}
