package store

import (
	"testing"

	"github.com/google/uuid"
)

func TestSelectKnowledgeContextsChoosesDocumentsThenSections(t *testing.T) {
	docA, docB, docC, docD := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	parent := func() uuid.UUID { return uuid.New() }
	items := []KnowledgeCandidate{
		{DocumentID: docA, ParentID: parent(), Title: "A1"},
		{DocumentID: docA, ParentID: parent(), Title: "A2"},
		{DocumentID: docB, ParentID: parent(), Title: "B1"},
		{DocumentID: docA, ParentID: parent(), Title: "A3"},
		{DocumentID: docC, ParentID: parent(), Title: "C1"},
		{DocumentID: docD, ParentID: parent(), Title: "D1"},
	}
	got := SelectKnowledgeContexts("比较几个文档", items, 5)
	if len(got) != 5 {
		t.Fatalf("got %d contexts, want 5", len(got))
	}
	want := []string{"A1", "B1", "C1", "A2", "A3"}
	for i := range want {
		if got[i].Title != want[i] || got[i].Rank != i+1 {
			t.Fatalf("context %d=%#v, want title=%s rank=%d", i, got[i], want[i], i+1)
		}
	}
}

func TestSelectKnowledgeContextsDeduplicatesParents(t *testing.T) {
	doc, parent := uuid.New(), uuid.New()
	got := SelectKnowledgeContexts("问题", []KnowledgeCandidate{
		{DocumentID: doc, ParentID: parent, Title: "first"},
		{DocumentID: doc, ParentID: parent, Title: "duplicate"},
	}, 5)
	if len(got) != 1 || got[0].Title != "first" {
		t.Fatalf("unexpected contexts: %#v", got)
	}
}

func TestSelectKnowledgeContextsStaysWithinExactTitleDocument(t *testing.T) {
	docA, docB := uuid.New(), uuid.New()
	items := []KnowledgeCandidate{
		{DocumentID: docA, ParentID: uuid.New(), Title: "鱼香肉丝的做法"},
		{DocumentID: docB, ParentID: uuid.New(), Title: "香干肉丝的做法"},
		{DocumentID: docA, ParentID: uuid.New(), Title: "鱼香肉丝的做法"},
	}
	got := SelectKnowledgeContexts("鱼香肉丝怎样炒？", items, 2)
	if len(got) != 2 || got[0].DocumentID != docA || got[1].DocumentID != docA {
		t.Fatalf("exact-title contexts crossed documents: %#v", got)
	}
}
