package workers

import (
	"testing"

	"cortex/backend/internal/blobstore"
)

func TestTopicFor(t *testing.T) {
	tests := map[string]string{
		"knowledge.document.uploaded": "cortex.knowledge.index.v1",
		"knowledge.document.parsed":   "cortex.document.parsed.v1",
		"search.projection.requested": "cortex.search.projection.v1",
		"audit.export.requested":      "cortex.audit.export.v1",
	}
	for eventType, want := range tests {
		if got := topicFor(eventType); got != want {
			t.Fatalf("topicFor(%q) = %q, want %q", eventType, got, want)
		}
	}
}

func TestObjectGCBackendUsesRecordedBackend(t *testing.T) {
	local, _ := blobstore.NewLocal(t.TempDir())
	minio, _ := blobstore.NewLocal(t.TempDir())

	got, err := objectGCBackend("local", local, minio)
	if err != nil || got != local {
		t.Fatalf("local backend = %#v, %v", got, err)
	}
	got, err = objectGCBackend("minio", local, minio)
	if err != nil || got != minio {
		t.Fatalf("minio backend = %#v, %v", got, err)
	}
	if _, err = objectGCBackend("minio", local, nil); err == nil {
		t.Fatal("unconfigured MinIO backend was accepted")
	}
	if _, err = objectGCBackend("unknown", local, minio); err == nil {
		t.Fatal("unknown backend was accepted")
	}
}
