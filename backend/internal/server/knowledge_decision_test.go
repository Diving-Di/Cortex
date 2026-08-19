package server

import "testing"

func TestDecideWeakKnowledgeEvidence(t *testing.T) {
	if got := decideWeakKnowledgeEvidence("上次那个结论是什么", false); got != knowledgeDecisionAmbiguous {
		t.Fatalf("got %q", got)
	}
	if got := decideWeakKnowledgeEvidence("比较两个候选范围", true); got != knowledgeDecisionScopeConflict {
		t.Fatalf("got %q", got)
	}
	if got := decideWeakKnowledgeEvidence("火星移民预算", false); got != knowledgeDecisionAbsent {
		t.Fatalf("got %q", got)
	}
}
