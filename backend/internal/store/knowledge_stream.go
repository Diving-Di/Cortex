package store

import (
	"context"
	"errors"

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
