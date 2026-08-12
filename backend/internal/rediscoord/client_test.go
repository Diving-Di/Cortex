package rediscoord

import (
	"bufio"
	"strings"
	"testing"
)

func TestAIEventKeysKeepClusterTagAndVersion(t *testing.T) {
	stable := AIEventKeys("event-1", "")
	versioned := AIEventKeys("event-1", "v123")
	if stable.ActiveVersion != "diary:ai-event:{event-1}:active_version" {
		t.Fatalf("active key=%q", stable.ActiveVersion)
	}
	for _, key := range versioned.DataKeys() {
		if !strings.Contains(key, "{event-1}") || !strings.HasSuffix(key, ":v123") {
			t.Fatalf("invalid version key %q", key)
		}
	}
}

func TestNew(t *testing.T) {
	client, err := New("redis://worker:secret@redis.internal:6380/2")
	if err != nil {
		t.Fatal(err)
	}
	if client.address != "redis.internal:6380" || client.username != "worker" || client.password != "secret" || client.database != 2 {
		t.Fatalf("unexpected client: %#v", client)
	}
	if _, err := New("http://localhost"); err == nil {
		t.Fatal("expected invalid scheme")
	}
}
func TestReadReply(t *testing.T) {
	value, err := readReply(bufio.NewReader(strings.NewReader(":2\r\n")))
	if err != nil || value != 2 {
		t.Fatalf("value=%d err=%v", value, err)
	}
	if _, err = readReply(bufio.NewReader(strings.NewReader("-ERR failed\r\n"))); err == nil {
		t.Fatal("expected Redis error")
	}
}
