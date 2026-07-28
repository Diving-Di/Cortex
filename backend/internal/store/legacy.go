package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

type LegacyMessage struct {
	ID        int32  `json:"id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

type Conversation struct {
	ID           int32           `json:"id"`
	Title        string          `json:"title"`
	SourceScope  string          `json:"source_scope"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
	Messages     []LegacyMessage `json:"messages,omitempty"`
	Version      int             `json:"version"`
	MessageCount int             `json:"message_count"`
	TotalTokens  int64           `json:"total_tokens"`
	Summary      *string         `json:"summary,omitempty"`
}

func (s *Store) ConversationSummaryInput(ctx context.Context, p domain.Principal, id int32) (*string, int, []LegacyMessage, error) {
	var summary *string
	var version int
	var messages []LegacyMessage
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT summary,summary_version FROM conversations WHERE tenant_id=$1 AND user_id=$2 AND id=$3`, p.TenantID, p.UserID, id).Scan(&summary, &version); errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("CONVERSATION_NOT_FOUND", "对话不存在", 404)
		} else if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id,role,content,created_at FROM messages WHERE tenant_id=$1 AND conversation_id=$2 ORDER BY id`, p.TenantID, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m LegacyMessage
			var at time.Time
			if err := rows.Scan(&m.ID, &m.Role, &m.Content, &at); err != nil {
				return err
			}
			m.CreatedAt = at.Format(time.RFC3339Nano)
			messages = append(messages, m)
		}
		return rows.Err()
	})
	return summary, version, messages, err
}
func (s *Store) SaveConversationSummary(ctx context.Context, p domain.Principal, id int32, summary string, through int32, version int, model string) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE conversations SET summary=$4,summary_through_message_id=$5,summary_version=summary_version+1,summary_model=$6,summary_updated_at=now()
	WHERE tenant_id=$1 AND user_id=$2 AND id=$3 AND summary_version=$7`, p.TenantID, p.UserID, id, summary, through, model, version)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apierror.New("CONVERSATION_SUMMARY_CONFLICT", "会话摘要已被更新", 409)
		}
		return nil
	})
}

func ValidSourceScope(scope string) bool {
	return scope == "knowledge" || scope == "growth" || scope == "all"
}

func (s *Store) CreateConversation(ctx context.Context, principal domain.Principal, title, sourceScope string) (Conversation, error) {
	var result Conversation
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var created, updated time.Time
		if err := tx.QueryRow(ctx, `INSERT INTO conversations (tenant_id,user_id,title,source_scope)
			VALUES ($1,$2,$3,$4) RETURNING id,title,source_scope,created_at,updated_at`,
			principal.TenantID, principal.UserID, title, sourceScope,
		).Scan(&result.ID, &result.Title, &result.SourceScope, &created, &updated); err != nil {
			return err
		}
		result.CreatedAt = created.Format(time.RFC3339Nano)
		result.UpdatedAt = updated.Format(time.RFC3339Nano)
		result.Messages = []LegacyMessage{}
		return nil
	})
	return result, err
}

func (s *Store) ListScopedConversations(ctx context.Context, principal domain.Principal, search, scope string, limit, offset int) ([]Conversation, int, error) {
	var result []Conversation
	var total int
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		where := `c.tenant_id=$1 AND c.user_id=$2 AND c.source_scope IN ('knowledge','growth','all')
		  AND ($3='' OR c.source_scope=$3) AND ($4='' OR c.title ILIKE '%'||$4||'%' OR EXISTS
		  (SELECT 1 FROM messages m WHERE m.tenant_id=c.tenant_id AND m.conversation_id=c.id AND m.content ILIKE '%'||$4||'%'))`
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM conversations c WHERE `+where,
			principal.TenantID, principal.UserID, scope, search).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT c.id,c.title,c.source_scope,c.created_at,c.updated_at,c.version,
		  (SELECT count(*) FROM messages m WHERE m.tenant_id=c.tenant_id AND m.conversation_id=c.id),
		  COALESCE((SELECT sum(input_tokens+output_tokens) FROM ai_usage_records a WHERE a.tenant_id=c.tenant_id AND a.conversation_id=c.id),0)
		  FROM conversations c WHERE `+where+` ORDER BY c.updated_at DESC,c.id DESC LIMIT $5 OFFSET $6`,
			principal.TenantID, principal.UserID, scope, search, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item Conversation
			var created, updated time.Time
			if err := rows.Scan(&item.ID, &item.Title, &item.SourceScope, &created, &updated, &item.Version, &item.MessageCount, &item.TotalTokens); err != nil {
				return err
			}
			item.CreatedAt = created.Format(time.RFC3339Nano)
			item.UpdatedAt = updated.Format(time.RFC3339Nano)
			result = append(result, item)
		}
		return rows.Err()
	})
	return result, total, err
}

func (s *Store) RenameConversation(ctx context.Context, principal domain.Principal, id int32, title string, version int) (Conversation, error) {
	var result Conversation
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var created, updated time.Time
		err := tx.QueryRow(ctx, `UPDATE conversations SET title=$4,version=version+1,updated_at=now()
		  WHERE tenant_id=$1 AND user_id=$2 AND id=$3 AND version=$5
		  RETURNING id,title,source_scope,created_at,updated_at,version`,
			principal.TenantID, principal.UserID, id, title, version).Scan(
			&result.ID, &result.Title, &result.SourceScope, &created, &updated, &result.Version)
		if errors.Is(err, pgx.ErrNoRows) {
			var exists bool
			_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM conversations WHERE tenant_id=$1 AND user_id=$2 AND id=$3)`,
				principal.TenantID, principal.UserID, id).Scan(&exists)
			if exists {
				return apierror.New("CONVERSATION_VERSION_CONFLICT", "会话已被更新，请刷新后重试", 409)
			}
			return apierror.New("CONVERSATION_NOT_FOUND", "对话不存在", 404)
		}
		if err != nil {
			return err
		}
		result.CreatedAt = created.Format(time.RFC3339Nano)
		result.UpdatedAt = updated.Format(time.RFC3339Nano)
		return auditResource(ctx, tx, principal, "conversation.rename", "conversation", fmt.Sprint(id))
	})
	return result, err
}

func (s *Store) GetConversation(ctx context.Context, principal domain.Principal, id int32) (Conversation, error) {
	var result Conversation
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var created, updated time.Time
		err := tx.QueryRow(ctx, `SELECT id,title,source_scope,created_at,updated_at FROM conversations
            WHERE tenant_id=$1 AND user_id=$2 AND id=$3`, principal.TenantID, principal.UserID, id,
		).Scan(&result.ID, &result.Title, &result.SourceScope, &created, &updated)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("CONVERSATION_NOT_FOUND", "对话不存在", 404)
		}
		if err != nil {
			return err
		}
		result.CreatedAt = created.Format(time.RFC3339Nano)
		result.UpdatedAt = updated.Format(time.RFC3339Nano)
		rows, err := tx.Query(ctx, `SELECT id,role,content,created_at FROM messages
            WHERE tenant_id=$1 AND conversation_id=$2 ORDER BY id`, principal.TenantID, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var message LegacyMessage
			var timestamp time.Time
			if err := rows.Scan(&message.ID, &message.Role, &message.Content, &timestamp); err != nil {
				return err
			}
			message.CreatedAt = timestamp.Format(time.RFC3339Nano)
			result.Messages = append(result.Messages, message)
		}
		if result.Messages == nil {
			result.Messages = []LegacyMessage{}
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) DeleteConversation(ctx context.Context, principal domain.Principal, id int32) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		command, err := tx.Exec(ctx, `DELETE FROM conversations WHERE tenant_id=$1 AND user_id=$2 AND id=$3`, principal.TenantID, principal.UserID, id)
		if err != nil {
			return err
		}
		if command.RowsAffected() == 0 {
			return apierror.New("CONVERSATION_NOT_FOUND", "对话不存在", 404)
		}
		return auditResource(ctx, tx, principal, "conversation.delete", "conversation", fmt.Sprint(id))
	})
}

func (s *Store) SetGeneratedConversationTitle(ctx context.Context, p domain.Principal, id int32, title string) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE conversations SET title=$4,version=version+1,updated_at=now()
		WHERE tenant_id=$1 AND user_id=$2 AND id=$3 AND version=1
		AND (SELECT count(*) FROM messages WHERE tenant_id=$1 AND conversation_id=$3)<=2`,
			p.TenantID, p.UserID, id, title)
		return err
	})
}
