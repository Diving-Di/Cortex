package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestEmbeddingClientForwardsAuthAndDimensions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer gateway-key" {
			t.Fatalf("Authorization = %q", got)
		}
		var body struct {
			Model          string   `json:"model"`
			Input          []string `json:"input"`
			Dimensions     int      `json:"dimensions"`
			EncodingFormat string   `json:"encoding_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "cortex-embedding" || len(body.Input) != 2 ||
			body.Dimensions != 3 || body.EncodingFormat != "float" {
			t.Fatalf("body = %#v", body)
		}
		_, _ = w.Write([]byte(`{"data":[` +
			`{"index":1,"embedding":[4,5,6]},` +
			`{"index":0,"embedding":[1,2,3]}]}`))
	}))
	defer server.Close()

	client := LocalEmbeddingClient{
		BaseURL: server.URL, APIKey: "gateway-key", Model: "cortex-embedding",
		Dimensions: 3, SendDimensions: true, HTTPClient: server.Client(),
	}
	result, err := client.Embed(context.Background(), []string{"中文", "English"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 || result[0][0] != 1 || result[1][0] != 4 {
		t.Fatalf("result = %#v", result)
	}
}

func TestEmbeddingClientRejectsWrongDimensions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[1,2]}]}`))
	}))
	defer server.Close()
	client := LocalEmbeddingClient{
		BaseURL: server.URL, Model: "test", Dimensions: 3, HTTPClient: server.Client(),
	}
	if _, err := client.Embed(context.Background(), []string{"test"}); err == nil {
		t.Fatal("wrong dimensions accepted")
	}
}

func TestEmbeddingClientBatchesAndRetriesTransientFailure(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := requests.Add(1)
		if call == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		var body struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[1]}`))
		for index := 1; index < len(body.Input); index++ {
			_, _ = w.Write([]byte(`,{"index":` + string(rune('0'+index)) + `,"embedding":[1]}`))
		}
		_, _ = w.Write([]byte(`]}`))
	}))
	defer server.Close()

	client := LocalEmbeddingClient{
		BaseURL: server.URL, Model: "test", Dimensions: 1, MaxBatchSize: 2,
		MaxRetries: 1, RetryBaseDelay: time.Millisecond, HTTPClient: server.Client(),
	}
	result, err := client.Embed(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 3 || requests.Load() != 3 {
		t.Fatalf("result=%d requests=%d", len(result), requests.Load())
	}
}

func TestRerankClientRejectsIncompleteAndOversizedResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"index":0,"relevance_score":0.9}]}`))
	}))
	defer server.Close()
	client := LocalRerankClient{BaseURL: server.URL, Model: "test", MaxDocuments: 2, HTTPClient: server.Client()}
	if _, err := client.Rerank(context.Background(), "q", []string{"a", "b"}); err == nil {
		t.Fatal("incomplete rerank response accepted")
	}
	if _, err := client.Rerank(context.Background(), "q", []string{"a", "b", "c"}); err == nil {
		t.Fatal("oversized rerank request accepted")
	}
}

func TestRerankClientRejectsDuplicateAndOutOfRangeIndexes(t *testing.T) {
	for name, body := range map[string]string{
		"duplicate":    `{"results":[{"index":0,"relevance_score":0.9},{"index":0,"relevance_score":0.8}]}`,
		"out_of_range": `{"results":[{"index":0,"relevance_score":0.9},{"index":2,"relevance_score":0.8}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			client := LocalRerankClient{BaseURL: server.URL, Model: "test", MaxDocuments: 2, HTTPClient: server.Client()}
			if _, err := client.Rerank(context.Background(), "q", []string{"a", "b"}); err == nil {
				t.Fatal("invalid rerank response accepted")
			}
		})
	}
}

func TestEmbeddingClientCancellationStopsRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := LocalEmbeddingClient{
		BaseURL: server.URL, Model: "test", Dimensions: 1,
		MaxRetries: 3, RetryBaseDelay: time.Second, HTTPClient: server.Client(),
	}
	if _, err := client.Embed(ctx, []string{"a"}); err == nil {
		t.Fatal("cancelled request succeeded")
	}
}
