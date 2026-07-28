package secretbox

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestSealBindsCiphertextToTenantAAD(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	box, err := New(1, key)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, nonce, err := box.Seal([]byte("sensitive-cookie"), []byte("tenant-a"))
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := box.Open(ciphertext, nonce, []byte("tenant-a"))
	if err != nil || string(plaintext) != "sensitive-cookie" {
		t.Fatalf("unexpected decrypt result %q %v", plaintext, err)
	}
	if _, err := box.Open(ciphertext, nonce, []byte("tenant-b")); err == nil {
		t.Fatal("ciphertext must not decrypt for another tenant")
	}
	if strings.Contains(string(ciphertext), "sensitive-cookie") {
		t.Fatal("ciphertext exposed plaintext")
	}
}

func TestNewRejectsWrongKeyLength(t *testing.T) {
	if _, err := New(1, base64.StdEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("short key must fail")
	}
}
