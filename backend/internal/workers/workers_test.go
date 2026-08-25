package workers

import "testing"

func TestTopicFor(t *testing.T) {
	tests := map[string]string{
		"knowledge.document.uploaded": "cortex.knowledge.index.v1",
		"search.projection.requested": "cortex.search.projection.v1",
		"audit.export.requested":      "cortex.audit.export.v1",
	}
	for eventType, want := range tests {
		if got := topicFor(eventType); got != want {
			t.Fatalf("topicFor(%q) = %q, want %q", eventType, got, want)
		}
	}
}
