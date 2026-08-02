package recipe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"diary-listener/backend/internal/store"
)

func TestRetrieverRejectsWrongEmbeddingDimensions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]}]}`))
	}))
	defer server.Close()
	retriever := &Retriever{
		Store:          &store.Store{},
		EmbeddingURL:   server.URL,
		EmbeddingModel: "iic/nlp_gte_sentence-embedding_chinese-small",
	}
	_, err := retriever.Search(context.Background(), "红烧鱼", 3)
	if err == nil || !strings.Contains(err.Error(), "dimensions") {
		t.Fatalf("expected dimensions error, got %v", err)
	}
}

func TestRetrieverRerankUsesReturnedOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"index":1,"relevance_score":0.9},{"index":0,"relevance_score":0.1}]}`))
	}))
	defer server.Close()
	retriever := &Retriever{RerankURL: server.URL, RerankModel: "BAAI/bge-reranker-v2-m3"}
	got, err := retriever.Rerank(context.Background(), "鱼", []store.RecipeCandidate{
		{DocumentID: 1, Snippet: "一"},
		{DocumentID: 2, Snippet: "二"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].DocumentID != 2 {
		t.Fatalf("unexpected order: %#v", got)
	}
	if got[0].RerankScore == nil || *got[0].RerankScore != 0.9 {
		t.Fatalf("unexpected rerank score: %#v", got[0].RerankScore)
	}
}

func TestFuseCandidatesCombinesRoutesAndIsDeterministic(t *testing.T) {
	routes := [][]store.RecipeCandidate{
		{{ChunkID: 2, Routes: []string{"vector"}}, {ChunkID: 1, Routes: []string{"vector"}}},
		{{ChunkID: 1, Routes: []string{"title"}}, {ChunkID: 2, Routes: []string{"title"}}},
	}
	got := fuseCandidates(routes, 60, 10)
	if len(got) != 2 || got[0].ChunkID != 1 || got[1].ChunkID != 2 {
		t.Fatalf("unexpected fusion: %#v", got)
	}
	if len(got[0].Routes) != 2 || got[0].FusionScore <= 0 {
		t.Fatalf("route provenance was not retained: %#v", got[0])
	}
}

func TestRetrieverRejectsIncompleteRerankResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"index":0,"relevance_score":0.9}]}`))
	}))
	defer server.Close()
	r := &Retriever{RerankURL: server.URL, RerankModel: "model"}
	_, err := r.Rerank(context.Background(), "鱼", []store.RecipeCandidate{{Snippet: "一"}, {Snippet: "二"}})
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("expected incomplete response error, got %v", err)
	}
}
