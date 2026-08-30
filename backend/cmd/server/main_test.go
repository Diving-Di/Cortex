package main

import (
	"net/http"
	"testing"
	"time"
)

func TestHTTPServerDoesNotTruncateStreamingResponses(t *testing.T) {
	server := newHTTPServer("127.0.0.1:0", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	if server.WriteTimeout != 0 {
		t.Fatalf("WriteTimeout = %s, want zero for SSE", server.WriteTimeout)
	}
	if server.ReadHeaderTimeout != 10*time.Second || server.IdleTimeout != 90*time.Second {
		t.Fatalf("unexpected defensive timeouts: header=%s idle=%s", server.ReadHeaderTimeout, server.IdleTimeout)
	}
}
