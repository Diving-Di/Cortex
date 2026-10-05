package eventbus

import (
	"context"
	"fmt"
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

func TestPublishRequiresBrokerAcknowledgement(t *testing.T) {
	for _, tc := range []struct {
		body string
		ok   bool
	}{
		{`{"offsets":[{"partition":0,"offset":12}]}`, true},
		{`{"offsets":[{"error_code":3,"error":"private upstream message"}]}`, false},
		{`{"offsets":[]}`, false},
		{`{}`, false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, tc.body) }))
			defer server.Close()
			err := NewKafkaREST(server.URL).Publish(context.Background(), "topic", "key", Event{})
			if (err == nil) != tc.ok {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestConsumerUsesConfiguredEndpointAndCleansFailedSubscription(t *testing.T) {
	for _, status := range []int{204, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			deleted := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.Path {
				case "POST /consumers/group":
					io.WriteString(w, `{"instance_id":"instance","base_uri":"http://unreachable.invalid:8082/consumers/group/instances/instance"}`)
				case "POST /consumers/group/instances/instance/subscription":
					w.WriteHeader(status)
				case "DELETE /consumers/group/instances/instance":
					deleted = true
					w.WriteHeader(204)
				default:
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			consumer, err := NewConsumer(context.Background(), server.URL, "group", []string{"topic"})
			if status == 204 {
				if err != nil {
					t.Fatal(err)
				}
				_ = consumer.Close(context.Background())
			} else if err == nil {
				t.Fatal("expected subscribe error")
			}
			if !deleted {
				t.Fatal("consumer not cleaned up")
			}
		})
	}
}
