package blobstore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestS3DeleteTargetsObjectVersion(t *testing.T) {
	var method, path, version string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, version = r.Method, r.URL.Path, r.URL.Query().Get("versionId")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	store, err := NewS3(server.URL, "private", "access", "secret", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), "tenants/t/file", "v 17"); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodDelete || path != "/private/tenants/t/file" || version != "v 17" {
		t.Fatalf("delete request = %s %s version=%q", method, path, version)
	}
}
