package store

import (
	"context"
	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
	"errors"
	"github.com/google/uuid"
	"strings"
	"sync"
	"testing"
)

func assertProductCode(t *testing.T, err error, code string) {
	t.Helper()
	var target *apierror.Error
	if !errors.As(err, &target) || target.Code != code {
		t.Fatalf("error=%v; want %s", err, code)
	}
}

func TestKnowledgeAnswerNoteOwnershipSourcesQuotaAndReplay(t *testing.T) {
	app, admin := gcTestPools(t)
	ctx := context.Background()
	user, tenant, noteID := gcTestTenant(t, ctx, admin, 1024)
	otherUser, otherTenant, _ := gcTestTenant(t, ctx, admin, 1024)
	p := domain.Principal{TenantID: tenant, UserID: user, TenantActive: true}
	other := domain.Principal{TenantID: otherTenant, UserID: otherUser, TenantActive: true}
	s := &Store{Pool: app, AdminPool: admin}
	documentID := uuid.New()
	_, err := admin.Exec(ctx, `INSERT INTO knowledge_documents(id,tenant_id,note_id,source_type,title,content_hash,active_index_version,status,knowledge_enabled) VALUES($1,$2,$3,'note','来源笔记',$4,1,'ready',true)`, documentID, tenant, noteID, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	candidate := KnowledgeCandidate{DocumentID: documentID, NoteID: &noteID, SourceType: "note", Title: "来源笔记", Content: "原始依据", IndexVersion: 1, Rank: 1}
	messageID, _, err := s.SaveKnowledgeAnswerOutcome(ctx, p, nil, "", "问题", "完整回答 [K1]", "complete", "", "", 1, []KnowledgeCandidate{candidate}, KnowledgeTraceConfig{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveKnowledgeNote(ctx, other, messageID)
	assertProductCode(t, err, "MESSAGE_NOT_FOUND")
	_, err = s.KnowledgeMessageSources(ctx, other, messageID)
	assertProductCode(t, err, "MESSAGE_NOT_FOUND")
	sources, err := s.KnowledgeMessageSources(ctx, p, messageID)
	if err != nil || len(sources) != 1 {
		t.Fatalf("sources=%v err=%v", sources, err)
	}
	if _, err = admin.Exec(ctx, `UPDATE tenants SET note_quota=2 WHERE id=$1`, tenant); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan domain.Note, 4)
	failures := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			note, err := s.SaveKnowledgeNote(ctx, p, messageID)
			results <- note
			failures <- err
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	var savedID int32
	for note := range results {
		if savedID != 0 && savedID != note.ID {
			t.Fatal("retry created duplicate notes")
		}
		savedID = note.ID
		if !strings.Contains(note.Content, "原始依据") || !strings.Contains(note.Content, "/notes/") {
			t.Fatal("source snapshot missing")
		}
	}
	failedID, _, err := s.SaveKnowledgeAnswerOutcome(ctx, p, nil, "", "问题", "片段", "failed", "FAILED", "generation", 1, []KnowledgeCandidate{candidate}, KnowledgeTraceConfig{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveKnowledgeNote(ctx, p, failedID)
	assertProductCode(t, err, "KNOWLEDGE_ANSWER_INCOMPLETE")
	anotherID, _, err := s.SaveKnowledgeAnswerOutcome(ctx, p, nil, "", "另一问题", "另一回答 [K1]", "complete", "", "", 1, []KnowledgeCandidate{candidate}, KnowledgeTraceConfig{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveKnowledgeNote(ctx, p, anotherID)
	assertProductCode(t, err, "NOTE_QUOTA_EXCEEDED")
	if _, err = admin.Exec(ctx, `UPDATE knowledge_documents SET knowledge_enabled=false WHERE id=$1`, documentID); err != nil {
		t.Fatal(err)
	}
	_, err = s.SaveKnowledgeNote(ctx, p, messageID)
	assertProductCode(t, err, "KNOWLEDGE_SOURCE_INVALID")
	sources, err = s.KnowledgeMessageSources(ctx, p, messageID)
	if err != nil || len(sources) != 0 {
		t.Fatalf("revoked sources=%v err=%v", sources, err)
	}
}

func TestRevisionRestoreChecksExpectedVersionAndPreservesHistory(t *testing.T) {
	app, admin := gcTestPools(t)
	ctx := context.Background()
	user, tenant, noteID := gcTestTenant(t, ctx, admin, 1024)
	p := domain.Principal{TenantID: tenant, UserID: user, TenantActive: true}
	s := &Store{Pool: app}
	original, err := s.GetNote(ctx, p, noteID)
	if err != nil {
		t.Fatal(err)
	}
	text := "第二版"
	updated, err := s.UpdateNote(ctx, p, noteID, domain.NotePatch{Content: &text, ExpectedUpdatedAt: &original.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	revisions, err := s.ListRevisions(ctx, p, noteID)
	if err != nil || len(revisions) != 1 {
		t.Fatalf("revisions=%v err=%v", revisions, err)
	}
	_, err = s.RestoreRevision(ctx, p, noteID, revisions[0].ID, &original.UpdatedAt)
	assertProductCode(t, err, "NOTE_VERSION_CONFLICT")
	restored, err := s.RestoreRevision(ctx, p, noteID, revisions[0].ID, &updated.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Content != original.Content {
		t.Fatal("wrong restored content")
	}
	revisions, err = s.ListRevisions(ctx, p, noteID)
	if err != nil || len(revisions) != 2 {
		t.Fatalf("history=%v err=%v", revisions, err)
	}
	otherUser, otherTenant, _ := gcTestTenant(t, ctx, admin, 1024)
	_, err = s.RestoreRevision(ctx, domain.Principal{TenantID: otherTenant, UserID: otherUser, TenantActive: true}, noteID, revisions[0].ID, nil)
	assertProductCode(t, err, "NOTE_NOT_FOUND")
}
