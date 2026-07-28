package store

import (
	"strings"
	"testing"
)

func TestKnowledgeLexicalQueryUsesSafeORTerms(t *testing.T) {
	value := knowledgeLexicalQuery("如何制定成长计划？ (2026) | test")
	if strings.ContainsAny(value, "?!():*'\\") {
		t.Fatalf("query contains tsquery control punctuation: %q", value)
	}
	if !strings.Contains(value, " | ") {
		t.Fatalf("query should use OR terms: %q", value)
	}
	if !strings.Contains(value, "成长") {
		t.Fatalf("query should include Chinese bigrams: %q", value)
	}
}

func TestKnowledgeLexicalQueryHasFallback(t *testing.T) {
	if got := knowledgeLexicalQuery("？！"); got != "cortex" {
		t.Fatalf("unexpected fallback: %q", got)
	}
}

func TestFilterKnowledgeSemanticCandidatesRequiresStrongSemanticOnlyEvidence(t *testing.T) {
	candidates := []KnowledgeCandidate{
		{ChildID: 1, Score: knowledgeSemanticOnlyMinimumScore - 0.01},
		{ChildID: 2, Score: knowledgeSemanticOnlyMinimumScore},
	}
	filtered := filterKnowledgeSemanticCandidates(candidates, false)
	if len(filtered) != 1 || filtered[0].ChildID != 2 {
		t.Fatalf("unexpected semantic-only candidates: %#v", filtered)
	}
}

func TestFilterKnowledgeSemanticCandidatesKeepsHybridCandidates(t *testing.T) {
	candidates := []KnowledgeCandidate{{ChildID: 1, Score: 0.1}}
	filtered := filterKnowledgeSemanticCandidates(candidates, true)
	if len(filtered) != 1 || filtered[0].ChildID != 1 {
		t.Fatalf("unexpected hybrid candidates: %#v", filtered)
	}
}
