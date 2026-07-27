package store

import (
	"context"
	"time"

	"diary-listener/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

// ExportNote is the stable exchange representation used by Markdown export.
// It is intentionally independent from the removed full-backup format.
type ExportNote struct {
	ID       int32
	Type     string
	Title    string
	Content  string
	NoteDate *string
	Summary  *string
}

func (s *Store) ExportNotes(ctx context.Context, principal domain.Principal) ([]ExportNote, error) {
	var result []ExportNote
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id,type,title,content,note_date,summary
			FROM notes WHERE tenant_id=$1 AND deleted_at IS NULL ORDER BY id`, principal.TenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item ExportNote
			var noteDate *time.Time
			if err := rows.Scan(
				&item.ID, &item.Type, &item.Title, &item.Content, &noteDate, &item.Summary,
			); err != nil {
				return err
			}
			if noteDate != nil {
				value := noteDate.Format(time.DateOnly)
				item.NoteDate = &value
			}
			result = append(result, item)
		}
		return rows.Err()
	})
	return result, err
}
