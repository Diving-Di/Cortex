package store

import (
	"context"
	"errors"
	"strings"

	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type KnowledgeCandidate struct {
	DocumentID                 uuid.UUID `json:"document_id"`
	NoteID                     *int32    `json:"note_id,omitempty"`
	SourceType, Title, Content string
	Heading                    []string
	IndexVersion, Rank         int
	Score                      float64
}

func (s *Store) ValidateKnowledgeCollections(ctx context.Context, p domain.Principal, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM knowledge_collections WHERE tenant_id=$1 AND id=ANY($2) AND deleted_at IS NULL`, p.TenantID, ids).Scan(&count); err != nil {
			return err
		}
		if count != len(ids) {
			return apierror.New("KNOWLEDGE_SCOPE_NOT_FOUND", "知识库范围不存在", 404)
		}
		return nil
	})
}
func (s *Store) SearchKnowledge(ctx context.Context, p domain.Principal, query string, embedding []float32, model string, collections []uuid.UUID, limit int) ([]KnowledgeCandidate, error) {
	var out []KnowledgeCandidate
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `WITH eligible AS (
SELECT c.id,c.parent_id,c.document_id,c.index_version,c.embedding_text,c.embedding,d.title,d.source_type,d.note_id
FROM knowledge_child_chunks c JOIN knowledge_documents d ON d.tenant_id=c.tenant_id AND d.id=c.document_id
LEFT JOIN notes n ON n.tenant_id=d.tenant_id AND n.id=d.note_id
WHERE c.tenant_id=$1 AND d.status='ready' AND d.deleted_at IS NULL AND d.knowledge_enabled
AND c.index_version=d.active_index_version AND c.embedding_model=$4 AND c.embedding IS NOT NULL
AND (coalesce(cardinality($3::uuid[]),0)=0 OR d.collection_id=ANY($3)) AND (d.source_type<>'note' OR n.deleted_at IS NULL)),
v AS (SELECT id,row_number() OVER(ORDER BY embedding <=> $2::vector) rank FROM eligible LIMIT 20),
f AS (SELECT id,row_number() OVER(ORDER BY ts_rank_cd(to_tsvector('simple',embedding_text),plainto_tsquery('simple',$5)) DESC) rank FROM eligible WHERE to_tsvector('simple',embedding_text) @@ plainto_tsquery('simple',$5) LIMIT 20),
score AS (SELECT id,sum(value) score FROM (SELECT id,1.0/(60+rank) value FROM v UNION ALL SELECT id,1.0/(60+rank) FROM f)x GROUP BY id ORDER BY score DESC LIMIT $6)
SELECT e.document_id,e.note_id,e.source_type,e.title,p.content,p.heading_path,e.index_version,score.score
FROM score JOIN eligible e ON e.id=score.id JOIN knowledge_parent_chunks p ON p.tenant_id=$1 AND p.id=e.parent_id ORDER BY score.score DESC`, p.TenantID, vectorLiteral(embedding), collections, model, query, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c KnowledgeCandidate
			if err := rows.Scan(&c.DocumentID, &c.NoteID, &c.SourceType, &c.Title, &c.Content, &c.Heading, &c.IndexVersion, &c.Score); err != nil {
				return err
			}
			c.Rank = len(out) + 1
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}
func (s *Store) SaveKnowledgeAnswer(ctx context.Context, p domain.Principal, conversationID *int32, requestID, question, answer string, sources []KnowledgeCandidate) (int32, int32, error) {
	var messageID, savedConversationID int32
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		if conversationID != nil {
			if err := tx.QueryRow(ctx, `SELECT id FROM conversations WHERE tenant_id=$1 AND user_id=$2 AND id=$3 AND source_scope='knowledge'`, p.TenantID, p.UserID, *conversationID).Scan(&savedConversationID); err != nil {
				return apierror.New("CONVERSATION_NOT_FOUND", "对话不存在", 404)
			}
		} else {
			title := strings.TrimSpace(question)
			r := []rune(title)
			if len(r) > 32 {
				title = string(r[:32]) + "…"
			}
			if title == "" {
				title = "知识问答"
			}
			if err := tx.QueryRow(ctx, `INSERT INTO conversations(tenant_id,user_id,title,source_scope) VALUES($1,$2,$3,'knowledge') RETURNING id`, p.TenantID, p.UserID, title).Scan(&savedConversationID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO messages(tenant_id,conversation_id,role,content,status,request_id) VALUES($1,$2,'user',$3,'complete',nullif($4,''))`, p.TenantID, savedConversationID, question, requestID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO messages(tenant_id,conversation_id,role,content,status) VALUES($1,$2,'assistant',$3,'complete') RETURNING id`, p.TenantID, savedConversationID, answer).Scan(&messageID); err != nil {
			return err
		}
		for _, src := range sources {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM knowledge_documents WHERE tenant_id=$1 AND id=$2 AND status='ready' AND deleted_at IS NULL AND active_index_version=$3)`, p.TenantID, src.DocumentID, src.IndexVersion).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return apierror.New("KNOWLEDGE_SOURCE_INVALID", "知识来源已失效", 409)
			}
			snippet := []rune(src.Content)
			if len(snippet) > 500 {
				snippet = snippet[:500]
			}
			if _, err := tx.Exec(ctx, `INSERT INTO knowledge_message_sources(tenant_id,message_id,source_type,document_id,note_id,title,snippet,index_version,rank) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, p.TenantID, messageID, src.SourceType, src.DocumentID, src.NoteID, src.Title, string(snippet), src.IndexVersion, src.Rank); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE conversations SET updated_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, savedConversationID)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		err = apierror.New("CONVERSATION_NOT_FOUND", "对话不存在", 404)
	}
	return messageID, savedConversationID, err
}
