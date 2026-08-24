package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type NoteFilter struct {
	Page      int
	PageSize  int
	Type      string
	StartDate *time.Time
	EndDate   *time.Time
	TagID     *int32
}

type NoteInput struct {
	Type     string
	Title    string
	Content  string
	NoteDate *time.Time
	Summary  *string
}

type NotePatch struct {
	Title             *string
	Content           *string
	NoteDate          *time.Time
	SetNoteDate       bool
	Summary           *string
	SetSummary        bool
	ExpectedUpdatedAt *time.Time
}

func setTenant(ctx context.Context, tx pgx.Tx, principal domain.Principal) error {
	_, err := tx.Exec(ctx, `SELECT set_config('app.current_tenant_id',$1,true)`, principal.TenantID.String())
	return err
}

func (s *Store) ListNotes(ctx context.Context, principal domain.Principal, filter NoteFilter) ([]domain.Note, int64, error) {
	var items []domain.Note
	var total int64
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		args := []any{principal.TenantID}
		where := []string{"n.tenant_id=$1", "n.deleted_at IS NULL"}
		if filter.Type != "" {
			args = append(args, filter.Type)
			where = append(where, fmt.Sprintf("n.type=$%d", len(args)))
		}
		if filter.StartDate != nil {
			args = append(args, *filter.StartDate)
			where = append(where, fmt.Sprintf("n.note_date >= $%d", len(args)))
		}
		if filter.EndDate != nil {
			args = append(args, *filter.EndDate)
			where = append(where, fmt.Sprintf("n.note_date <= $%d", len(args)))
		}
		join := ""
		if filter.TagID != nil {
			join = " JOIN note_tags nt ON nt.note_id=n.id AND nt.tenant_id=n.tenant_id"
			args = append(args, *filter.TagID)
			where = append(where, fmt.Sprintf("nt.tag_id=$%d", len(args)))
		}
		base := " FROM notes n" + join + " WHERE " + strings.Join(where, " AND ")
		if err := tx.QueryRow(ctx, "SELECT count(*)"+base, args...).Scan(&total); err != nil {
			return err
		}
		args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
		rows, err := tx.Query(ctx, `SELECT n.id,n.type,n.title,n.content,n.note_date,n.summary,
            n.word_count,n.created_at,n.updated_at`+base+
			fmt.Sprintf(" ORDER BY n.note_date DESC NULLS LAST,n.updated_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)),
			args...,
		)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			note, err := scanNote(rows)
			if err != nil {
				return err
			}
			items = append(items, note)
		}
		return rows.Err()
	})
	return items, total, err
}

func (s *Store) CreateNote(ctx context.Context, principal domain.Principal, input NoteInput) (domain.Note, error) {
	var result domain.Note
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var quota, count int64
		if err := tx.QueryRow(ctx, `SELECT note_quota FROM tenants WHERE id=$1`, principal.TenantID).Scan(&quota); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM notes WHERE tenant_id=$1 AND deleted_at IS NULL`, principal.TenantID).Scan(&count); err != nil {
			return err
		}
		if count >= quota {
			return apierror.New("NOTE_QUOTA_EXCEEDED", "笔记数量已达到配额", 409)
		}
		row := tx.QueryRow(ctx, `INSERT INTO notes
            (tenant_id,created_by,updated_by,type,title,content,note_date,summary,word_count)
            VALUES ($1,$2,$2,$3,$4,$5,$6,$7,$8)
            RETURNING id,type,title,content,note_date,summary,word_count,created_at,updated_at`,
			principal.TenantID, principal.UserID, input.Type, input.Title, input.Content,
			input.NoteDate, input.Summary, wordCount(input.Content),
		)
		note, err := scanNote(row)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return apierror.New("PERIOD_NOTE_EXISTS", "该周期笔记已存在", 409)
			}
			return err
		}
		result = note
		return audit(ctx, tx, principal, "note.create", result.ID)
	})
	return result, err
}

func (s *Store) GetNote(ctx context.Context, principal domain.Principal, noteID int32) (domain.Note, error) {
	var result domain.Note
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		note, err := getNoteTx(ctx, tx, principal, noteID)
		result = note
		return err
	})
	return result, err
}

func (s *Store) UpdateNote(ctx context.Context, principal domain.Principal, noteID int32, patch NotePatch) (domain.Note, error) {
	var result domain.Note
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		note, err := getNoteTx(ctx, tx, principal, noteID)
		if err != nil {
			return err
		}
		if patch.ExpectedUpdatedAt != nil && !note.UpdatedAt.Equal(*patch.ExpectedUpdatedAt) {
			return apierror.New("NOTE_CONFLICT", "笔记已被其他请求更新", 409)
		}
		title, content, noteDate, summary := note.Title, note.Content, note.NoteDate, note.Summary
		if patch.Title != nil {
			title = strings.TrimSpace(*patch.Title)
			if title == "" {
				return apierror.New("TITLE_REQUIRED", "标题不能为空", 422)
			}
		}
		if patch.Content != nil {
			content = *patch.Content
			if content != note.Content {
				if _, err := tx.Exec(ctx, `INSERT INTO note_revisions
                    (tenant_id,note_id,created_by,content,reason) VALUES ($1,$2,$3,$4,'update')`,
					principal.TenantID, note.ID, principal.UserID, note.Content,
				); err != nil {
					return err
				}
			}
		}
		if patch.SetNoteDate {
			noteDate = patch.NoteDate
		}
		if patch.SetSummary {
			summary = patch.Summary
		}
		row := tx.QueryRow(ctx, `UPDATE notes SET title=$1,content=$2,note_date=$3,summary=$4,
            word_count=$5,updated_by=$6,updated_at=now()
            WHERE tenant_id=$7 AND id=$8
            RETURNING id,type,title,content,note_date,summary,word_count,created_at,updated_at`,
			title, content, noteDate, summary, wordCount(content), principal.UserID, principal.TenantID, noteID,
		)
		result, err = scanNote(row)
		if err != nil {
			return err
		}
		return audit(ctx, tx, principal, "note.update", noteID)
	})
	return result, err
}

func (s *Store) DeleteNote(ctx context.Context, principal domain.Principal, noteID int32) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		if _, err := getNoteTx(ctx, tx, principal, noteID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE notes SET deleted_at=now() WHERE tenant_id=$1 AND id=$2`, principal.TenantID, noteID); err != nil {
			return err
		}
		return audit(ctx, tx, principal, "note.delete", noteID)
	})
}

func (s *Store) ListRevisions(ctx context.Context, principal domain.Principal, noteID int32) ([]domain.Revision, error) {
	var revisions []domain.Revision
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		if _, err := getNoteTx(ctx, tx, principal, noteID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id,note_id,content,reason,created_at
            FROM note_revisions WHERE tenant_id=$1 AND note_id=$2 ORDER BY created_at DESC`,
			principal.TenantID, noteID,
		)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var revision domain.Revision
			if err := rows.Scan(&revision.ID, &revision.NoteID, &revision.Content, &revision.Reason, &revision.CreatedAt); err != nil {
				return err
			}
			revisions = append(revisions, revision)
		}
		return rows.Err()
	})
	return revisions, err
}

func (s *Store) RestoreRevision(ctx context.Context, principal domain.Principal, noteID, revisionID int32) (domain.Note, error) {
	var result domain.Note
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		note, err := getNoteTx(ctx, tx, principal, noteID)
		if err != nil {
			return err
		}
		var content string
		err = tx.QueryRow(ctx, `SELECT content FROM note_revisions
            WHERE tenant_id=$1 AND note_id=$2 AND id=$3`,
			principal.TenantID, noteID, revisionID,
		).Scan(&content)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("REVISION_NOT_FOUND", "历史版本不存在", 404)
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO note_revisions
            (tenant_id,note_id,created_by,content,reason) VALUES ($1,$2,$3,$4,'before_restore')`,
			principal.TenantID, noteID, principal.UserID, note.Content,
		); err != nil {
			return err
		}
		result, err = scanNote(tx.QueryRow(ctx, `UPDATE notes SET content=$1,word_count=$2,
            updated_by=$3,updated_at=now() WHERE tenant_id=$4 AND id=$5
            RETURNING id,type,title,content,note_date,summary,word_count,created_at,updated_at`,
			content, wordCount(content), principal.UserID, principal.TenantID, noteID,
		))
		if err != nil {
			return err
		}
		return audit(ctx, tx, principal, "note.revision.restore", noteID)
	})
	return result, err
}

type scanner interface{ Scan(...any) error }

func scanNote(row scanner) (domain.Note, error) {
	var note domain.Note
	err := row.Scan(&note.ID, &note.Type, &note.Title, &note.Content, &note.NoteDate,
		&note.Summary, &note.WordCount, &note.CreatedAt, &note.UpdatedAt)
	return note, err
}

func getNoteTx(ctx context.Context, tx pgx.Tx, principal domain.Principal, noteID int32) (domain.Note, error) {
	note, err := scanNote(tx.QueryRow(ctx, `SELECT id,type,title,content,note_date,summary,
        word_count,created_at,updated_at FROM notes
        WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, principal.TenantID, noteID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Note{}, apierror.New("NOTE_NOT_FOUND", "笔记不存在", 404)
	}
	return note, err
}

func audit(ctx context.Context, tx pgx.Tx, principal domain.Principal, action string, noteID int32) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_logs
        (tenant_id,user_id,action,resource_type,resource_id) VALUES ($1,$2,$3,'note',$4)`,
		principal.TenantID, principal.UserID, action, fmt.Sprint(noteID),
	)
	return err
}

func wordCount(value string) int32 {
	var count int32
	for _, r := range value {
		if !unicode.IsSpace(r) {
			count++
		}
	}
	return count
}
