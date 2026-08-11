package store

import (
	"context"
	"errors"
	"strings"

	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

type KnowledgeRequestResult struct {
	MessageID      int32
	ConversationID int32
	Content        string
	Status         string
	ErrorCode      string
	UpstreamStage  string
	OutputTokens   int
}

func (s *Store) GetKnowledgeRequest(ctx context.Context, p domain.Principal, requestID string) (KnowledgeRequestResult, bool, error) {
	if requestID == "" {
		return KnowledgeRequestResult{}, false, nil
	}
	var result KnowledgeRequestResult
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT a.id, a.conversation_id, a.content, a.status,
			       coalesce(a.error_code,''), coalesce(a.upstream_stage,''), a.output_tokens
			FROM messages u
			JOIN conversations c ON c.tenant_id=u.tenant_id AND c.id=u.conversation_id
			JOIN LATERAL (
				SELECT m.id,m.conversation_id,m.content,m.status,m.error_code,m.upstream_stage,m.output_tokens
				FROM messages m
				WHERE m.tenant_id=u.tenant_id AND m.conversation_id=u.conversation_id
				  AND m.role='assistant' AND m.id>u.id
				ORDER BY m.id LIMIT 1
			) a ON true
			WHERE u.tenant_id=$1 AND c.user_id=$2 AND c.source_scope='knowledge'
			  AND u.role='user' AND u.request_id=$3`, p.TenantID, p.UserID, requestID).Scan(
			&result.MessageID, &result.ConversationID, &result.Content, &result.Status,
			&result.ErrorCode, &result.UpstreamStage, &result.OutputTokens,
		)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return KnowledgeRequestResult{}, false, nil
	}
	return result, err == nil, err
}

func (s *Store) SaveKnowledgeAnswerOutcome(ctx context.Context, p domain.Principal, conversationID *int32, requestID, question, answer, status, errorCode, upstreamStage string, outputTokens int, sources []KnowledgeCandidate) (int32, int32, error) {
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
		if err := tx.QueryRow(ctx, `INSERT INTO messages(tenant_id,conversation_id,role,content,status,error_code,upstream_stage,output_tokens) VALUES($1,$2,'assistant',$3,$4,nullif($5,''),nullif($6,''),$7) RETURNING id`, p.TenantID, savedConversationID, answer, status, errorCode, upstreamStage, outputTokens).Scan(&messageID); err != nil {
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
