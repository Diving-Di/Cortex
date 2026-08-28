package store

import (
	"context"
	"time"

	"cortex/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

// ExportNote is the stable exchange representation used by Markdown export.
// ExportNotes returns the tenant's active notes for Markdown exchange export.
func (s *Store) ExportNotes(ctx context.Context, principal domain.Principal) ([]domain.ExportNote, error) {
	var result []domain.ExportNote
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
			var item domain.ExportNote
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
