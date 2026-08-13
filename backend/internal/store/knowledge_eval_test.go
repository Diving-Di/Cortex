package store

import "testing"

func TestCanonicalEvalCaseIsStableAcrossInputOrder(t *testing.T) {
	_, first, err := canonicalEvalCase("case-1", "query", "answer", []string{"b", "a"}, []string{"z", "x"})
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := canonicalEvalCase("case-1", "query", "answer", []string{"a", "b"}, []string{"x", "z"})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("hash differs: %s != %s", first, second)
	}
}

func TestCanonicalEvalCaseChangesWithEvidence(t *testing.T) {
	_, first, _ := canonicalEvalCase("case-1", "query", "answer", []string{"a"}, nil)
	_, second, _ := canonicalEvalCase("case-1", "query", "answer", []string{"b"}, nil)
	if first == second {
		t.Fatal("evidence change did not change case hash")
	}
}
