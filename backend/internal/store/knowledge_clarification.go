package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type KnowledgeClarification struct {
	ID                uuid.UUID   `json:"clarification_id"`
	ConversationID    int32       `json:"conversation_id"`
	OriginalRequestID string      `json:"-"`
	OriginalQuestion  string      `json:"-"`
	CollectionIDs     []uuid.UUID `json:"-"`
	Kind              string      `json:"kind"`
	Prompt            string      `json:"prompt"`
	ExpiresAt         time.Time   `json:"expires_at"`
	AlreadyResumed    bool        `json:"-"`
}

func (s *Store) CreateKnowledgeClarification(ctx context.Context, p domain.Principal, conversationID *int32, requestID, question string, collectionIDs []uuid.UUID, kind, prompt string, ttl time.Duration) (KnowledgeClarification, error) {
	var value KnowledgeClarification
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		if conversationID != nil {
			if err := tx.QueryRow(ctx, `SELECT id FROM conversations WHERE tenant_id=$1 AND user_id=$2 AND id=$3 AND source_scope='knowledge'`, p.TenantID, p.UserID, *conversationID).Scan(&value.ConversationID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return apierror.New("KNOWLEDGE_SCOPE_NOT_FOUND", "知识库资源不存在", 404)
				}
				return err
			}
		} else {
			title := strings.TrimSpace(question)
			if runes := []rune(title); len(runes) > 80 {
				title = string(runes[:80])
			}
			if err := tx.QueryRow(ctx, `INSERT INTO conversations(tenant_id,user_id,title,source_scope) VALUES($1,$2,$3,'knowledge') RETURNING id`, p.TenantID, p.UserID, title).Scan(&value.ConversationID); err != nil {
				return err
			}
		}
		return tx.QueryRow(ctx, `INSERT INTO knowledge_clarifications(tenant_id,user_id,conversation_id,original_request_id,original_question,collection_ids,kind,prompt,expires_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,now()+$9::interval)
			ON CONFLICT(tenant_id,user_id,original_request_id) DO UPDATE SET prompt=excluded.prompt
			RETURNING id,conversation_id,kind,prompt,expires_at`, p.TenantID, p.UserID, value.ConversationID, requestID, question, collectionIDs, kind, prompt, ttl.String()).Scan(&value.ID, &value.ConversationID, &value.Kind, &value.Prompt, &value.ExpiresAt)
	})
	return value, err
}

func (s *Store) ConsumeKnowledgeClarification(ctx context.Context, p domain.Principal, id uuid.UUID) (KnowledgeClarification, error) {
	var value KnowledgeClarification
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `UPDATE knowledge_clarifications SET status='resumed',resumed_at=now()
			WHERE id=$1 AND tenant_id=$2 AND user_id=$3 AND status='pending' AND expires_at>now()
			RETURNING id,conversation_id,original_request_id,original_question,collection_ids,kind,prompt,expires_at`, id, p.TenantID, p.UserID).Scan(&value.ID, &value.ConversationID, &value.OriginalRequestID, &value.OriginalQuestion, &value.CollectionIDs, &value.Kind, &value.Prompt, &value.ExpiresAt)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `SELECT id,conversation_id,original_request_id,original_question,collection_ids,kind,prompt,expires_at
				FROM knowledge_clarifications WHERE id=$1 AND tenant_id=$2 AND user_id=$3 AND status='resumed'`, id, p.TenantID, p.UserID).Scan(&value.ID, &value.ConversationID, &value.OriginalRequestID, &value.OriginalQuestion, &value.CollectionIDs, &value.Kind, &value.Prompt, &value.ExpiresAt)
			if err == nil {
				value.AlreadyResumed = true
				return nil
			}
			if errors.Is(err, pgx.ErrNoRows) {
				return apierror.New("KNOWLEDGE_CLARIFICATION_NOT_FOUND", "澄清请求不存在或已过期", 404)
			}
		}
		return err
	})
	return value, err
}
