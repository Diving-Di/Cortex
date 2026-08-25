package eventbus

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConsumerCommitUsesEmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/consumer/offsets" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if len(body) != 0 {
			t.Fatalf("commit body = %q, want empty", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	consumer := &Consumer{instance: server.URL + "/consumer", client: server.Client()}
	if err := consumer.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestConsumerCloseDeletesInstance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/consumer" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	consumer := &Consumer{instance: server.URL + "/consumer", client: server.Client()}
	if err := consumer.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}
