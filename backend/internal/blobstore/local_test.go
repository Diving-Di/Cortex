package blobstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"
)

func TestLocalRoundTripAndTraversal(t *testing.T) {
	s, _ := NewLocal(t.TempDir())
	data := []byte("cortex")
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	info, err := s.Put(context.Background(), "tenants/t/attachments/a", bytes.NewReader(data), int64(len(data)), digest)
	if err != nil || info.SHA256 != digest {
		t.Fatalf("put: %#v %v", info, err)
	}
	r, _, err := s.Open(context.Background(), "tenants/t/attachments/a")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	r.Close()
	if !bytes.Equal(got, data) {
		t.Fatal("content mismatch")
	}
	if _, err = s.Stat(context.Background(), "../escape"); err == nil {
		t.Fatal("traversal accepted")
	}
}
func TestLocalRejectsChecksumMismatch(t *testing.T) {
	s, _ := NewLocal(t.TempDir())
	if _, err := s.Put(context.Background(), "safe", bytes.NewReader([]byte("x")), 1, "bad"); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
}
