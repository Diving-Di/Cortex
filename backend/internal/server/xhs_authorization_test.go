package server

import (
	"path/filepath"
	"testing"
)

func TestSafeRuntimePath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "runtime", "xhs-auth")
	valid := filepath.ToSlash(filepath.Join("runtime", "xhs-auth", "tenant", "attempt", "qr.png"))
	path, err := safeRuntimePath(root, valid)
	if err != nil || filepath.Base(path) != "qr.png" {
		t.Fatalf("expected valid QR path, path=%q err=%v", path, err)
	}
	for _, value := range []string{"../secret", "runtime/xhs-auth/../../secret", "/etc/passwd"} {
		if _, err := safeRuntimePath(root, value); err == nil {
			t.Fatalf("expected path %q to be rejected", value)
		}
	}
}
