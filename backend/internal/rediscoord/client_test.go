package rediscoord

import (
	"bufio"
	"strings"
	"testing"
)

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
