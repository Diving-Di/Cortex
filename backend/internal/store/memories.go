package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type GrowthMemory struct {
	ID           int64     `json:"id"`
	Category     string    `json:"category"`
	Content      string    `json:"content"`
	Importance   int       `json:"importance"`
	SourceType   string    `json:"source_type"`
	CreationMode string    `json:"creation_mode"`
	Version      int       `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func ValidMemoryCategory(v string) bool {
	return v == "fact" || v == "preference" || v == "goal" || v == "habit" || v == "milestone"
}

func (s *Store) ListGrowthMemories(ctx context.Context, p domain.Principal, category, search string, limit, offset int) ([]GrowthMemory, int, error) {
	var items []GrowthMemory
	var total int
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		where := `tenant_id=$1 AND user_id=$2 AND deleted_at IS NULL AND ($3='' OR category=$3) AND ($4='' OR content ILIKE '%'||$4||'%')`
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM growth_memories WHERE `+where, p.TenantID, p.UserID, category, search).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id,category,content,importance,source_type,creation_mode,version,created_at,updated_at
		  FROM growth_memories WHERE `+where+` ORDER BY importance DESC,updated_at DESC LIMIT $5 OFFSET $6`,
			p.TenantID, p.UserID, category, search, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v GrowthMemory
			if err := rows.Scan(&v.ID, &v.Category, &v.Content, &v.Importance, &v.SourceType, &v.CreationMode, &v.Version, &v.CreatedAt, &v.UpdatedAt); err != nil {
				return err
			}
			items = append(items, v)
		}
		return rows.Err()
	})
	return items, total, err
}
func (s *Store) CreateGrowthMemory(ctx context.Context, p domain.Principal, v GrowthMemory) (GrowthMemory, error) {
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO growth_memories(tenant_id,user_id,category,content,importance,source_type,creation_mode)
		  VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,version,created_at,updated_at`,
			p.TenantID, p.UserID, v.Category, v.Content, v.Importance, v.SourceType, v.CreationMode).Scan(&v.ID, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	})
	return v, err
}
func (s *Store) UpdateGrowthMemory(ctx context.Context, p domain.Principal, id int64, v GrowthMemory) (GrowthMemory, error) {
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `UPDATE growth_memories SET category=$4,content=$5,importance=$6,version=version+1,updated_at=now()
		  WHERE tenant_id=$1 AND user_id=$2 AND id=$3 AND version=$7 AND deleted_at IS NULL
		  RETURNING id,category,content,importance,source_type,creation_mode,version,created_at,updated_at`,
			p.TenantID, p.UserID, id, v.Category, v.Content, v.Importance, v.Version).Scan(&v.ID, &v.Category, &v.Content, &v.Importance, &v.SourceType, &v.CreationMode, &v.Version, &v.CreatedAt, &v.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("MEMORY_VERSION_CONFLICT", "记忆已被更新，请刷新后重试", 409)
		}
		return err
	})
	return v, err
}
func (s *Store) DeleteGrowthMemory(ctx context.Context, p domain.Principal, id int64) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE growth_memories SET deleted_at=now(),updated_at=now()
	  WHERE tenant_id=$1 AND user_id=$2 AND id=$3 AND deleted_at IS NULL`, p.TenantID, p.UserID, id)
		if err == nil && tag.RowsAffected() == 0 {
			return apierror.New("MEMORY_NOT_FOUND", "成长记忆不存在", 404)
		}
		return err
	})
}

type MemorySettings struct {
	SuggestionEnabled bool     `json:"suggestion_enabled"`
	AllowedCategories []string `json:"allowed_categories"`
	MinimumImportance int      `json:"minimum_importance"`
	ExcludedNoteTypes []string `json:"excluded_note_types"`
	ExcludedTagIDs    []int32  `json:"excluded_tag_ids"`
	RetentionDays     *int     `json:"retention_days"`
}

type MemoryDraftItem struct {
	Category   string `json:"category"`
	Content    string `json:"content"`
	Importance int    `json:"importance"`
	SourceType string `json:"source_type"`
	SourceID   int32  `json:"source_id"`
}
type MemoryDraft struct {
	ID        uuid.UUID         `json:"draft_id"`
	Items     []MemoryDraftItem `json:"items"`
	Status    string            `json:"status"`
	ExpiresAt time.Time         `json:"expires_at"`
}

func (s *Store) CreateMemoryDraft(ctx context.Context, p domain.Principal, items []MemoryDraftItem) (MemoryDraft, error) {
	v := MemoryDraft{ID: uuid.New(), Items: items, Status: "pending", ExpiresAt: time.Now().Add(30 * time.Minute)}
	raw, _ := json.Marshal(items)
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO growth_memory_drafts(id,tenant_id,user_id,items,expires_at) VALUES($1,$2,$3,$4,$5)`, v.ID, p.TenantID, p.UserID, raw, v.ExpiresAt)
		return err
	})
	return v, err
}
func (s *Store) CompleteMemoryDraft(ctx context.Context, p domain.Principal, id uuid.UUID, items []MemoryDraftItem, reject bool) ([]GrowthMemory, error) {
	var created []GrowthMemory
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		var status string
		var expires time.Time
		if err := tx.QueryRow(ctx, `SELECT status,expires_at FROM growth_memory_drafts WHERE tenant_id=$1 AND user_id=$2 AND id=$3 FOR UPDATE`, p.TenantID, p.UserID, id).Scan(&status, &expires); errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("MEMORY_DRAFT_NOT_FOUND", "记忆草稿不存在", 404)
		} else if err != nil {
			return err
		}
		if status != "pending" {
			return apierror.New("MEMORY_DRAFT_ALREADY_COMPLETED", "记忆草稿已经处理", 409)
		}
		if time.Now().After(expires) {
			_, _ = tx.Exec(ctx, `UPDATE growth_memory_drafts SET status='expired',completed_at=now() WHERE id=$1`, id)
			return apierror.New("MEMORY_DRAFT_EXPIRED", "记忆草稿已过期", 409)
		}
		if reject {
			_, err := tx.Exec(ctx, `UPDATE growth_memory_drafts SET status='rejected',completed_at=now() WHERE id=$1`, id)
			return err
		}
		for _, item := range items {
			if !ValidMemoryCategory(item.Category) || item.Importance < 1 || item.Importance > 10 || len([]rune(item.Content)) < 1 || len([]rune(item.Content)) > 5000 {
				return apierror.Validation(nil)
			}
			var noteID, conversationID, messageID *int32
			switch item.SourceType {
			case "note":
				var ok bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notes WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL)`, p.TenantID, item.SourceID).Scan(&ok); err != nil || !ok {
					if err != nil {
						return err
					}
					return apierror.New("SOURCE_NOT_FOUND", "来源不存在", 404)
				}
				noteID = &item.SourceID
			case "conversation":
				var ok bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM conversations WHERE tenant_id=$1 AND user_id=$2 AND id=$3)`, p.TenantID, p.UserID, item.SourceID).Scan(&ok); err != nil || !ok {
					if err != nil {
						return err
					}
					return apierror.New("SOURCE_NOT_FOUND", "来源不存在", 404)
				}
				conversationID = &item.SourceID
			case "message":
				var ok bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM messages m JOIN conversations c ON c.tenant_id=m.tenant_id AND c.id=m.conversation_id WHERE m.tenant_id=$1 AND m.id=$2 AND c.user_id=$3)`, p.TenantID, item.SourceID, p.UserID).Scan(&ok); err != nil || !ok {
					if err != nil {
						return err
					}
					return apierror.New("SOURCE_NOT_FOUND", "来源不存在", 404)
				}
				messageID = &item.SourceID
			default:
				return apierror.Validation(nil)
			}
			var v GrowthMemory
			err := tx.QueryRow(ctx, `INSERT INTO growth_memories(tenant_id,user_id,category,content,importance,source_type,source_note_id,source_conversation_id,source_message_id,creation_mode)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'ai_confirmed') RETURNING id,category,content,importance,source_type,creation_mode,version,created_at,updated_at`,
				p.TenantID, p.UserID, item.Category, item.Content, item.Importance, item.SourceType, noteID, conversationID, messageID).Scan(&v.ID, &v.Category, &v.Content, &v.Importance, &v.SourceType, &v.CreationMode, &v.Version, &v.CreatedAt, &v.UpdatedAt)
			if err != nil {
				return err
			}
			created = append(created, v)
		}
		raw, _ := json.Marshal(items)
		_, err := tx.Exec(ctx, `UPDATE growth_memory_drafts SET items=$2,status='confirmed',completed_at=now() WHERE id=$1`, id, raw)
		return err
	})
	return created, err
}

func (s *Store) GetMemorySettings(ctx context.Context, p domain.Principal) (MemorySettings, error) {
	v := MemorySettings{AllowedCategories: []string{"fact", "preference", "goal", "habit", "milestone"}, MinimumImportance: 5, ExcludedNoteTypes: []string{}, ExcludedTagIDs: []int32{}}
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT suggestion_enabled,allowed_categories,minimum_importance,excluded_note_types,excluded_tag_ids,retention_days FROM memory_settings WHERE tenant_id=$1`, p.TenantID).Scan(&v.SuggestionEnabled, &v.AllowedCategories, &v.MinimumImportance, &v.ExcludedNoteTypes, &v.ExcludedTagIDs, &v.RetentionDays)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	return v, err
}
func (s *Store) SaveMemorySettings(ctx context.Context, p domain.Principal, v MemorySettings) (MemorySettings, error) {
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO memory_settings(tenant_id,user_id,suggestion_enabled,allowed_categories,minimum_importance,excluded_note_types,excluded_tag_ids,retention_days)
VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(tenant_id) DO UPDATE SET suggestion_enabled=excluded.suggestion_enabled,allowed_categories=excluded.allowed_categories,minimum_importance=excluded.minimum_importance,excluded_note_types=excluded.excluded_note_types,excluded_tag_ids=excluded.excluded_tag_ids,retention_days=excluded.retention_days,updated_at=now()`,
			p.TenantID, p.UserID, v.SuggestionEnabled, v.AllowedCategories, v.MinimumImportance, v.ExcludedNoteTypes, v.ExcludedTagIDs, v.RetentionDays)
		return err
	})
	return v, err
}

func (s *Store) MemoryDraftSource(ctx context.Context, p domain.Principal, sourceType string, id int32) (string, error) {
	var text string
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		var err error
		switch sourceType {
		case "note":
			err = tx.QueryRow(ctx, `SELECT left(content,20000) FROM notes WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, p.TenantID, id).Scan(&text)
		case "conversation":
			err = tx.QueryRow(ctx, `SELECT left(string_agg(role||': '||content,E'\n' ORDER BY id),20000) FROM messages WHERE tenant_id=$1 AND conversation_id=$2`, p.TenantID, id).Scan(&text)
		case "message":
			err = tx.QueryRow(ctx, `SELECT left(m.content,20000) FROM messages m JOIN conversations c ON c.tenant_id=m.tenant_id AND c.id=m.conversation_id WHERE m.tenant_id=$1 AND m.id=$2 AND c.user_id=$3`, p.TenantID, id, p.UserID).Scan(&text)
		default:
			return apierror.Validation(nil)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("SOURCE_NOT_FOUND", "来源不存在", 404)
		}
		return err
	})
	return text, err
}
