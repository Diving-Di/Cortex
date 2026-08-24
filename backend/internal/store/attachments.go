package store

import (
	"context"
	"errors"
	"time"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

type Attachment struct {
	ID             int32     `json:"id"`
	NoteID         int32     `json:"note_id"`
	OriginalName   string    `json:"original_name"`
	StoredPath     string    `json:"-"`
	StorageBackend string    `json:"-"`
	ObjectKey      string    `json:"-"`
	ObjectVersion  string    `json:"-"`
	ETag           string    `json:"-"`
	MIMEType       string    `json:"mime_type"`
	Size           int64     `json:"size"`
	SHA256         string    `json:"sha256"`
	CreatedAt      time.Time `json:"-"`
}

type AttachmentResponse struct {
	ID           int32  `json:"id"`
	NoteID       int32  `json:"note_id"`
	OriginalName string `json:"original_name"`
	MIMEType     string `json:"mime_type"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256"`
	CreatedAt    string `json:"created_at"`
}

func (a Attachment) Response() AttachmentResponse {
	return AttachmentResponse{
		ID: a.ID, NoteID: a.NoteID, OriginalName: a.OriginalName,
		MIMEType: a.MIMEType, Size: a.Size, SHA256: a.SHA256,
		CreatedAt: a.CreatedAt.Format(time.RFC3339Nano),
	}
}

func (s *Store) AddAttachment(ctx context.Context, principal domain.Principal, item Attachment) (Attachment, error) {
	var result Attachment
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		if _, err := getNoteTx(ctx, tx, principal, item.NoteID); err != nil {
			return err
		}
		var quota, used int64
		if err := tx.QueryRow(ctx, `SELECT attachment_quota_bytes FROM tenants WHERE id=$1 FOR UPDATE`, principal.TenantID).Scan(&quota); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(size),0) FROM attachments WHERE tenant_id=$1`, principal.TenantID).Scan(&used); err != nil {
			return err
		}
		if used+item.Size > quota {
			return apierror.New("ATTACHMENT_QUOTA_EXCEEDED", "附件空间配额不足", 409)
		}
		return tx.QueryRow(ctx, `INSERT INTO attachments
			(tenant_id,uploaded_by,note_id,original_name,stored_path,storage_backend,object_key,object_version,etag,mime_type,size,sha256)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			RETURNING id,note_id,original_name,stored_path,storage_backend,coalesce(object_key,''),coalesce(object_version,''),coalesce(etag,''),mime_type,size,sha256,created_at`,
			principal.TenantID, principal.UserID, item.NoteID, item.OriginalName,
			item.StoredPath, item.StorageBackend, item.ObjectKey, item.ObjectVersion, item.ETag, item.MIMEType, item.Size, item.SHA256,
		).Scan(
			&result.ID, &result.NoteID, &result.OriginalName, &result.StoredPath,
			&result.StorageBackend, &result.ObjectKey, &result.ObjectVersion, &result.ETag,
			&result.MIMEType, &result.Size, &result.SHA256, &result.CreatedAt,
		)
	})
	return result, err
}

func (s *Store) GetAttachment(ctx context.Context, principal domain.Principal, attachmentID int32) (Attachment, error) {
	var result Attachment
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var err error
		result, err = getAttachmentTx(ctx, tx, principal, attachmentID)
		return err
	})
	return result, err
}

func (s *Store) ListAttachments(ctx context.Context, principal domain.Principal, noteID int32) ([]Attachment, error) {
	var result []Attachment
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		if _, err := getNoteTx(ctx, tx, principal, noteID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id,note_id,original_name,stored_path,storage_backend,coalesce(object_key,''),coalesce(object_version,''),coalesce(etag,''),mime_type,size,sha256,created_at
			FROM attachments WHERE tenant_id=$1 AND note_id=$2 AND deleted_at IS NULL ORDER BY id`, principal.TenantID, noteID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item Attachment
			if err := rows.Scan(
				&item.ID, &item.NoteID, &item.OriginalName, &item.StoredPath,
				&item.StorageBackend, &item.ObjectKey, &item.ObjectVersion, &item.ETag,
				&item.MIMEType, &item.Size, &item.SHA256, &item.CreatedAt,
			); err != nil {
				return err
			}
			result = append(result, item)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) DeleteAttachment(ctx context.Context, principal domain.Principal, attachmentID int32) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var backend, key, version string
		err := tx.QueryRow(ctx, `UPDATE attachments SET deleted_at=now() WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL RETURNING storage_backend,coalesce(object_key,stored_path),coalesce(object_version,'')`, principal.TenantID, attachmentID).Scan(&backend, &key, &version)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apierror.New("ATTACHMENT_NOT_FOUND", "附件不存在", 404)
			}
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO object_gc_jobs(tenant_id,storage_backend,object_key,object_version) VALUES($1,$2,$3,nullif($4,'')) ON CONFLICT DO NOTHING`, principal.TenantID, backend, key, version)
		return err
	})
}

func getAttachmentTx(ctx context.Context, tx pgx.Tx, principal domain.Principal, attachmentID int32) (Attachment, error) {
	var item Attachment
	err := tx.QueryRow(ctx, `SELECT id,note_id,original_name,stored_path,storage_backend,coalesce(object_key,''),coalesce(object_version,''),coalesce(etag,''),mime_type,size,sha256,created_at
		FROM attachments WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, principal.TenantID, attachmentID,
	).Scan(
		&item.ID, &item.NoteID, &item.OriginalName, &item.StoredPath,
		&item.StorageBackend, &item.ObjectKey, &item.ObjectVersion, &item.ETag,
		&item.MIMEType, &item.Size, &item.SHA256, &item.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, apierror.New("ATTACHMENT_NOT_FOUND", "附件不存在", 404)
	}
	return item, err
}
