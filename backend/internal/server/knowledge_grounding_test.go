package server

import (
	"testing"

	"diary-listener/backend/internal/store"
)

func TestFilterRerankEvidenceUsesOnlyConfiguredThreshold(t *testing.T) {
	items := []store.KnowledgeCandidate{{Score: 0.8}, {Score: 0.4}, {Score: 0.1}}
	if got := filterRerankEvidence(append([]store.KnowledgeCandidate(nil), items...), nil); len(got) != 3 {
		t.Fatalf("unset threshold filtered %d candidates", len(got))
	}
	threshold := 0.4
	got := filterRerankEvidence(append([]store.KnowledgeCandidate(nil), items...), &threshold)
	if len(got) != 2 || got[1].Score != 0.4 {
		t.Fatalf("qualified=%#v", got)
	}
}

func TestRerankMarginGate(t *testing.T) {
	items := []store.KnowledgeCandidate{{Score: 0.8}, {Score: 0.75}}
	threshold := 0.1
	if !rerankMarginTooSmall(items, &threshold) || rerankMarginTooSmall(items, nil) {
		t.Fatal("margin gate did not respect configured threshold")
	}
}
