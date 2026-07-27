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
