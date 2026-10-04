package store

import (
	"context"
	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"strings"
)

// SaveKnowledgeNote copies only a completed, owned answer with currently valid sources.
// Clients cannot substitute content or source identifiers.
func (s *Store) SaveKnowledgeNote(ctx context.Context, p domain.Principal, messageID int32) (domain.Note, error) {
	var result domain.Note
	err := s.WithPrincipalTx(ctx, p, func(tx pgx.Tx) error {
		var title, content, status string
		err := tx.QueryRow(ctx, `SELECT c.title,m.content,m.status FROM messages m
   JOIN conversations c ON c.tenant_id=m.tenant_id AND c.id=m.conversation_id
   WHERE m.tenant_id=$1 AND c.user_id=$2 AND c.source_scope='knowledge' AND m.id=$3 AND m.role='assistant'`, p.TenantID, p.UserID, messageID).Scan(&title, &content, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("MESSAGE_NOT_FOUND", "回答不存在", 404)
		}
		if err != nil {
			return err
		}
		if status != "complete" || strings.TrimSpace(content) == "" {
			return apierror.New("KNOWLEDGE_ANSWER_INCOMPLETE", "只能保存完整回答", 409)
		}
		rows, err := tx.Query(ctx, `SELECT s.rank,s.title,s.snippet,s.note_id,coalesce(s.document_id::text,''),
    (NOT s.source_deleted AND d.id IS NOT NULL AND d.deleted_at IS NULL AND d.knowledge_enabled AND d.status='ready' AND d.active_index_version=s.index_version
     AND (s.note_id IS NULL OR (n.id IS NOT NULL AND n.deleted_at IS NULL)))
    FROM knowledge_message_sources s
    LEFT JOIN knowledge_documents d ON d.tenant_id=s.tenant_id AND d.id=s.document_id
    LEFT JOIN notes n ON n.tenant_id=s.tenant_id AND n.id=s.note_id
    WHERE s.tenant_id=$1 AND s.message_id=$2 ORDER BY s.rank`, p.TenantID, messageID)
		if err != nil {
			return err
		}
		var appendix strings.Builder
		sourceCount := 0
		for rows.Next() {
			var rank int
			var sourceTitle, snippet, docID string
			var noteID *int32
			var valid bool
			if err := rows.Scan(&rank, &sourceTitle, &snippet, &noteID, &docID, &valid); err != nil {
				rows.Close()
				return err
			}
			if !valid {
				rows.Close()
				return apierror.New("KNOWLEDGE_SOURCE_INVALID", "来源已失效，请重新提问", 409)
			}
			fmt.Fprintf(&appendix, "\n\n### [K%d] %s\n\n%s\n", rank, sourceTitle, snippet)
			if noteID != nil {
				fmt.Fprintf(&appendix, "\n[来源笔记](/notes/%d)\n", *noteID)
			} else {
				fmt.Fprintf(&appendix, "\n文档标识：%s\n", docID)
			}
			sourceCount++
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if sourceCount == 0 {
			return apierror.New("KNOWLEDGE_NO_EVIDENCE", "回答没有有效来源", 422)
		}
		var quota, count int64
		if err := tx.QueryRow(ctx, `SELECT note_quota FROM tenants WHERE id=$1 FOR UPDATE`, p.TenantID).Scan(&quota); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM notes WHERE tenant_id=$1 AND deleted_at IS NULL`, p.TenantID).Scan(&count); err != nil {
			return err
		}
		var existingID int32

		err = tx.QueryRow(ctx, `SELECT n.id FROM notes n JOIN audit_logs a ON a.tenant_id=n.tenant_id AND split_part(a.resource_id,':',2)=n.id::text WHERE n.tenant_id=$1 AND a.user_id=$2 AND a.action='knowledge.answer.save' AND a.resource_type='knowledge_answer' AND split_part(a.resource_id,':',1)=$3 AND n.deleted_at IS NULL LIMIT 1`, p.TenantID, p.UserID, fmt.Sprint(messageID)).Scan(&existingID)
		if err == nil {
			result, err = getNoteTx(ctx, tx, p, existingID)
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if count >= quota {
			return apierror.New("NOTE_QUOTA_EXCEEDED", "笔记数量已达到配额", 409)
		}
		content += "\n\n## 来源快照" + appendix.String()
		result, err = scanNote(tx.QueryRow(ctx, `INSERT INTO notes(tenant_id,created_by,updated_by,type,title,content,word_count,note_date)
    VALUES($1,$2,$2,'normal',$3,$4,$5,CURRENT_DATE) RETURNING id,type,title,content,note_date,summary,word_count,created_at,updated_at`, p.TenantID, p.UserID, "知识问答："+title, content, wordCount(content)))
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO audit_logs(tenant_id,user_id,action,resource_type,resource_id) VALUES($1,$2,'knowledge.answer.save','knowledge_answer',$3)`, p.TenantID, p.UserID, fmt.Sprintf("%d:%d", messageID, result.ID))
		return err
	})
	return result, err
}

func (s *Store) KnowledgeMessageSources(ctx context.Context, p domain.Principal, messageID int32) ([]map[string]any, error) {
	items := make([]map[string]any, 0)
	err := s.WithPrincipalTx(ctx, p, func(tx pgx.Tx) error {
		var owned bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM messages m JOIN conversations c ON c.tenant_id=m.tenant_id AND c.id=m.conversation_id
    WHERE m.tenant_id=$1 AND c.user_id=$2 AND m.id=$3 AND c.source_scope='knowledge' AND m.role='assistant')`, p.TenantID, p.UserID, messageID).Scan(&owned); err != nil {
			return err
		}
		if !owned {
			return apierror.New("MESSAGE_NOT_FOUND", "回答不存在", 404)
		}
		rows, err := tx.Query(ctx, `SELECT s.rank,s.title,s.snippet,s.note_id,coalesce(s.document_id::text,''),s.source_type,s.index_version
   FROM knowledge_message_sources s JOIN knowledge_documents d ON d.tenant_id=s.tenant_id AND d.id=s.document_id
   WHERE s.tenant_id=$1 AND s.message_id=$2 AND d.deleted_at IS NULL AND d.knowledge_enabled AND d.status='ready' AND d.active_index_version=s.index_version AND NOT s.source_deleted AND (s.note_id IS NULL OR EXISTS(SELECT 1 FROM notes n WHERE n.tenant_id=s.tenant_id AND n.id=s.note_id AND n.deleted_at IS NULL)) ORDER BY s.rank`, p.TenantID, messageID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var rank, version int
			var title, snippet, documentID, sourceType string
			var noteID *int32
			if err := rows.Scan(&rank, &title, &snippet, &noteID, &documentID, &sourceType, &version); err != nil {
				return err
			}
			items = append(items, map[string]any{"citation": fmt.Sprintf("K%d", rank), "title": title, "snippet": snippet, "document_id": documentID, "note_id": noteID, "source_type": sourceType, "rank": rank, "index_version": version, "heading": []string{}})
		}
		return rows.Err()
	})
	return items, err
}
