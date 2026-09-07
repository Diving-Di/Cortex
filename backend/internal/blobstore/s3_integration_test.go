package blobstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestS3VersionDeletionIntegration(t *testing.T) {
	endpoint := os.Getenv("MINIO_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("MINIO_TEST_ENDPOINT is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s, err := NewS3(endpoint, "gc-test-"+uuid.NewString(), os.Getenv("MINIO_TEST_ACCESS_KEY"), os.Getenv("MINIO_TEST_SECRET_KEY"), false)
	if err != nil {
		t.Fatal(err)
	}
	response, err := s.do(ctx, http.MethodPut, "", "", nil, 0, emptySHA256)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		response, err := s.do(cleanupCtx, http.MethodDelete, "", "", nil, 0, emptySHA256)
		if err != nil {
			t.Errorf("remove test bucket: %v", err)
			return
		}
		response.Body.Close()
	})

	body := `<VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Status>Enabled</Status></VersioningConfiguration>`
	digest := sha256.Sum256([]byte(body))
	versionURL := s.objectURL("")
	versionURL.RawQuery = "versioning="
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, versionURL.String(), strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	s.sign(request, hex.EncodeToString(digest[:]), time.Now())
	response, err = s.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("enable bucket versioning: status %d", response.StatusCode)
	}

	key := "tenants/test/attachments/versioned"
	put := func(text string) ObjectInfo {
		t.Helper()
		data := []byte(text)
		digest := sha256.Sum256(data)
		info, err := s.Put(ctx, key, bytes.NewReader(data), int64(len(data)), hex.EncodeToString(digest[:]))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			// Cleanup runs after the test body; use a fresh context.
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cleanupCancel()
			_ = s.Delete(cleanupCtx, key, info.VersionID)
		})
		if info.VersionID == "" {
			t.Fatal("versioned upload did not return a version ID")
		}
		return info
	}
	old := put("old")
	latest := put("latest")
	if old.VersionID == latest.VersionID {
		t.Fatal("two writes returned the same version ID")
	}
	for i := 0; i < 2; i++ {
		if err := s.Delete(ctx, key, old.VersionID); err != nil {
			t.Fatalf("delete old version (attempt %d): %v", i+1, err)
		}
	}
	reader, info, err := s.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || string(data) != "latest" || info.VersionID != latest.VersionID {
		t.Fatalf("latest version was affected: version=%s readErr=%v", info.VersionID, err)
	}
	if err := s.Delete(ctx, key, latest.VersionID); err != nil {
		t.Fatal(err)
	}
}
