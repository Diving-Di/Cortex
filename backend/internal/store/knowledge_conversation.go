package store

import (
	"context"
	"errors"

	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

type KnowledgeConversationMessage struct {
	Role    string
	Content string
}

// LoadKnowledgeConversation loads only completed user/assistant turns from a
// knowledge conversation owned by the current principal. Failed and pending
// assistant turns are deliberately excluded from conversational memory.
func (s *Store) LoadKnowledgeConversation(ctx context.Context, p domain.Principal, conversationID int32, turnLimit int) ([]KnowledgeConversationMessage, error) {
	if turnLimit <= 0 {
		return nil, nil
	}
	var messages []KnowledgeConversationMessage
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(
			SELECT 1 FROM conversations
			WHERE tenant_id=$1 AND user_id=$2 AND id=$3 AND source_scope='knowledge'
		)`, p.TenantID, p.UserID, conversationID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apierror.New("CONVERSATION_NOT_FOUND", "对话不存在", 404)
		}
		rows, err := tx.Query(ctx, `WITH complete_turns AS (
			SELECT u.id AS user_id,u.content AS user_content,a.id AS assistant_id,a.content AS assistant_content
			FROM messages u
			JOIN LATERAL (
				SELECT m.id,m.content FROM messages m
				WHERE m.tenant_id=u.tenant_id AND m.conversation_id=u.conversation_id
				  AND m.role='assistant' AND m.id>u.id
				ORDER BY m.id LIMIT 1
			) a ON true
			WHERE u.tenant_id=$1 AND u.conversation_id=$2 AND u.role='user'
			  AND u.status='complete'
			  AND (SELECT status FROM messages WHERE tenant_id=$1 AND id=a.id)='complete'
			ORDER BY u.id DESC LIMIT $3
		)
		SELECT role,content FROM (
			SELECT user_id AS id,'user'::text AS role,user_content AS content FROM complete_turns
			UNION ALL
			SELECT assistant_id AS id,'assistant'::text AS role,assistant_content AS content FROM complete_turns
		) history ORDER BY id`, p.TenantID, conversationID, turnLimit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item KnowledgeConversationMessage
			if err := rows.Scan(&item.Role, &item.Content); err != nil {
				return err
			}
			messages = append(messages, item)
		}
		return rows.Err()
	})
	if errors.Is(err, pgx.ErrNoRows) {
		err = apierror.New("CONVERSATION_NOT_FOUND", "对话不存在", 404)
	}
	return messages, err
}
