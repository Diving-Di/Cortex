package auth

import (
	"strings"
	"testing"
)

func TestPasswordHashIsPythonCompatible(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "pbkdf2_sha256$260000$") {
		t.Fatalf("unexpected hash format: %s", hash)
	}
	if !VerifyPassword("correct-horse-battery", hash) {
		t.Fatal("generated password hash did not verify")
	}
	if VerifyPassword("wrong-password", hash) {
		t.Fatal("wrong password verified")
	}
}

func TestVerifyKnownPythonHash(t *testing.T) {
	// hashlib.pbkdf2_hmac("sha256", b"password", b"001122", 1).hex()
	const pythonHash = "pbkdf2_sha256$1$001122$f450a9fef2753ae80fbba3d7dc84cc887f82df168551b5d99e37dad9a4b9cc1f"
	if !VerifyPassword("password", pythonHash) {
		t.Fatal("Go verifier rejected a Python-format hash")
	}
}

func TestTokenHashNeverStoresRawToken(t *testing.T) {
	token, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || HashToken(token) == token || len(HashToken(token)) != 64 {
		t.Fatal("unexpected token/hash representation")
	}
}
