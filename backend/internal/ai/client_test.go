package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICompatibleStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("missing bearer token")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()

	client := &OpenAICompatibleClient{
		BaseURL: upstream.URL, APIKey: "test-key", HTTPClient: upstream.Client(),
	}
	events, err := client.StreamChat(context.Background(), ChatRequest{
		Model: "test", Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	for event := range events {
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		output.WriteString(event.Content)
	}
	if output.String() != "你好" {
		t.Fatalf("stream output = %q", output.String())
	}
}

func TestOpenAICompatibleRequiresKey(t *testing.T) {
	client := &OpenAICompatibleClient{}
	if _, err := client.StreamChat(context.Background(), ChatRequest{}); err == nil {
		t.Fatal("missing key accepted")
	}
}

func TestOpenAICompatibleForwardsTracingMetadata(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Request-ID"); got != "req-123" {
			t.Fatalf("X-Request-ID = %q", got)
		}
		var body struct {
			Metadata RequestMetadata `json:"metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Metadata.RequestType != "report" || body.Metadata.Tenant != "tenant-hash" {
			t.Fatalf("metadata = %#v", body.Metadata)
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()
	client := &OpenAICompatibleClient{BaseURL: upstream.URL, APIKey: "key", HTTPClient: upstream.Client()}
	ctx := WithRequestMetadata(context.Background(), RequestMetadata{
		RequestID: "req-123", RequestType: "report", Tenant: "tenant-hash", Environment: "test",
	})
	events, err := client.StreamChat(ctx, ChatRequest{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}
}
