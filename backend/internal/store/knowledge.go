package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type KnowledgeCollection struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"-"`
	UpdatedAt   time.Time `json:"-"`
}

type KnowledgeCollectionResponse struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Version     int     `json:"version"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

func (c KnowledgeCollection) Response() KnowledgeCollectionResponse {
	return KnowledgeCollectionResponse{
		ID: c.ID, Name: c.Name, Description: c.Description, Version: c.Version,
		CreatedAt: c.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt: c.UpdatedAt.Format(time.RFC3339Nano),
	}
}

type KnowledgeDocument struct {
	ID               int64
	CollectionID     *int64
	OriginalName     string
	StoredPath       string
	MIMEType         string
	Extension        string
	Size             int64
	SHA256           string
	Status           string
	PageCount        *int
	CharacterCount   int64
	ParentChunkCount int
	ChildChunkCount  int
	Language         *string
	ErrorCode        *string
	ErrorMessage     *string
	IndexVersion     int
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

type KnowledgeDocumentResponse struct {
	ID               int64   `json:"id"`
	CollectionID     *int64  `json:"collection_id"`
	OriginalName     string  `json:"original_name"`
	MIMEType         string  `json:"mime_type"`
	Extension        string  `json:"extension"`
	Size             int64   `json:"size"`
	SHA256           string  `json:"sha256"`
	Status           string  `json:"status"`
	PageCount        *int    `json:"page_count"`
	CharacterCount   int64   `json:"character_count"`
	ParentChunkCount int     `json:"parent_chunk_count"`
	ChildChunkCount  int     `json:"child_chunk_count"`
	Language         *string `json:"language"`
	ErrorCode        *string `json:"error_code"`
	ErrorMessage     *string `json:"error_message"`
	IndexVersion     int     `json:"index_version"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

func (d KnowledgeDocument) Response() KnowledgeDocumentResponse {
	return KnowledgeDocumentResponse{
		ID: d.ID, CollectionID: d.CollectionID, OriginalName: d.OriginalName,
		MIMEType: d.MIMEType, Extension: d.Extension, Size: d.Size, SHA256: d.SHA256,
		Status: d.Status, PageCount: d.PageCount, CharacterCount: d.CharacterCount,
		ParentChunkCount: d.ParentChunkCount, ChildChunkCount: d.ChildChunkCount,
		Language: d.Language, ErrorCode: d.ErrorCode, ErrorMessage: d.ErrorMessage,
		IndexVersion: d.IndexVersion, CreatedAt: d.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt: d.UpdatedAt.Format(time.RFC3339Nano),
	}
}

func (s *Store) CreateKnowledgeCollection(
	ctx context.Context, principal domain.Principal, name string, description *string,
) (KnowledgeCollection, error) {
	var result KnowledgeCollection
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `INSERT INTO knowledge_collections
			(tenant_id,created_by,name,description) VALUES ($1,$2,$3,$4)
			RETURNING id,name,description,version,created_at,updated_at`,
			principal.TenantID, principal.UserID, name, description,
		).Scan(&result.ID, &result.Name, &result.Description, &result.Version, &result.CreatedAt, &result.UpdatedAt)
		if isUniqueViolation(err) {
			return apierror.New("COLLECTION_EXISTS", "知识集合名称已存在", 409)
		}
		return err
	})
	return result, err
}

func (s *Store) ListKnowledgeCollections(
	ctx context.Context, principal domain.Principal,
) ([]KnowledgeCollection, error) {
	var result []KnowledgeCollection
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id,name,description,version,created_at,updated_at
			FROM knowledge_collections WHERE tenant_id=$1 AND deleted_at IS NULL
			ORDER BY lower(name),id`, principal.TenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item KnowledgeCollection
			if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Version, &item.CreatedAt, &item.UpdatedAt); err != nil {
				return err
			}
			result = append(result, item)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) AddKnowledgeDocument(
	ctx context.Context, principal domain.Principal, item KnowledgeDocument,
) (KnowledgeDocument, error) {
	var result KnowledgeDocument
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		if item.CollectionID != nil {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(
				SELECT 1 FROM knowledge_collections
				WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL)`,
				principal.TenantID, *item.CollectionID,
			).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return apierror.New("COLLECTION_NOT_FOUND", "知识集合不存在", 404)
			}
		}
		var quota, used int64
		if err := tx.QueryRow(ctx, `SELECT knowledge_quota_bytes FROM tenants
			WHERE id=$1 FOR UPDATE`, principal.TenantID).Scan(&quota); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(size),0) FROM knowledge_documents
			WHERE tenant_id=$1 AND deleted_at IS NULL`, principal.TenantID).Scan(&used); err != nil {
			return err
		}
		if used+item.Size > quota {
			return apierror.New("DOCUMENT_QUOTA_EXCEEDED", "知识库空间配额不足", 409)
		}
		err := scanKnowledgeDocument(tx.QueryRow(ctx, `INSERT INTO knowledge_documents
			(tenant_id,uploaded_by,collection_id,original_name,stored_path,mime_type,extension,size,sha256)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			RETURNING id,collection_id,original_name,stored_path,mime_type,extension,size,sha256,
				status,page_count,character_count,parent_chunk_count,child_chunk_count,language,
				error_code,error_message,index_version,created_at,updated_at,deleted_at`,
			principal.TenantID, principal.UserID, item.CollectionID, item.OriginalName,
			item.StoredPath, item.MIMEType, item.Extension, item.Size, item.SHA256,
		), &result)
		if isUniqueViolation(err) {
			return apierror.New("DOCUMENT_DUPLICATE", "该文件已经存在于知识库", 409)
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO knowledge_index_jobs
			(tenant_id,document_id,target_index_version) VALUES ($1,$2,$3)`,
			principal.TenantID, result.ID, result.IndexVersion)
		return err
	})
	return result, err
}

func (s *Store) ListKnowledgeDocuments(
	ctx context.Context, principal domain.Principal, collectionID *int64, search, status string, limit, offset int,
) ([]KnowledgeDocument, int64, error) {
	var result []KnowledgeDocument
	var total int64
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		args := []any{principal.TenantID}
		where := "tenant_id=$1 AND deleted_at IS NULL"
		if collectionID != nil {
			args = append(args, *collectionID)
			where += fmt.Sprintf(" AND collection_id=$%d", len(args))
		}
		if search != "" {
			args = append(args, "%"+search+"%")
			where += fmt.Sprintf(" AND original_name ILIKE $%d", len(args))
		}
		if status != "" {
			args = append(args, status)
			where += fmt.Sprintf(" AND status=$%d", len(args))
		}
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM knowledge_documents WHERE "+where, args...).Scan(&total); err != nil {
			return err
		}
		args = append(args, limit, offset)
		rows, err := tx.Query(ctx, `SELECT id,collection_id,original_name,stored_path,mime_type,extension,size,sha256,
			status,page_count,character_count,parent_chunk_count,child_chunk_count,language,
			error_code,error_message,index_version,created_at,updated_at,deleted_at
			FROM knowledge_documents WHERE `+where+
			fmt.Sprintf(" ORDER BY created_at DESC,id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item KnowledgeDocument
			if err := scanKnowledgeDocument(rows, &item); err != nil {
				return err
			}
			result = append(result, item)
		}
		return rows.Err()
	})
	return result, total, err
}

func (s *Store) DeleteKnowledgeCollection(ctx context.Context, principal domain.Principal, collectionID int64) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM knowledge_documents
			WHERE tenant_id=$1 AND collection_id=$2 AND deleted_at IS NULL`,
			principal.TenantID, collectionID).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return apierror.New("COLLECTION_NOT_EMPTY", "集合中仍有文件，请先移动或删除文件", 409)
		}
		command, err := tx.Exec(ctx, `UPDATE knowledge_collections SET deleted_at=now(),updated_at=now()
			WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, principal.TenantID, collectionID)
		if err != nil {
			return err
		}
		if command.RowsAffected() == 0 {
			return apierror.New("COLLECTION_NOT_FOUND", "知识集合不存在", 404)
		}
		return nil
	})
}

func (s *Store) ReindexKnowledgeDocument(ctx context.Context, principal domain.Principal, documentID int64) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var version int
		err := tx.QueryRow(ctx, `UPDATE knowledge_documents SET status='uploaded',
			error_code=NULL,error_message=NULL,index_version=index_version+1,updated_at=now()
			WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL
			RETURNING index_version`, principal.TenantID, documentID).Scan(&version)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("DOCUMENT_NOT_FOUND", "知识文件不存在", 404)
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO knowledge_index_jobs
			(tenant_id,document_id,target_index_version) VALUES ($1,$2,$3)`,
			principal.TenantID, documentID, version)
		return err
	})
}

func (s *Store) KnowledgeDocumentPreview(
	ctx context.Context, principal domain.Principal, documentID int64,
) (string, error) {
	var preview string
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT left(p.content,4000)
			FROM knowledge_documents d
			JOIN knowledge_parent_chunks p ON p.tenant_id=d.tenant_id AND p.document_id=d.id
				AND p.index_version=d.index_version
			WHERE d.tenant_id=$1 AND d.id=$2 AND d.status='ready' AND d.deleted_at IS NULL
			ORDER BY p.parent_index LIMIT 1`, principal.TenantID, documentID).Scan(&preview)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("DOCUMENT_PREVIEW_UNAVAILABLE", "文件尚未完成提取或来源已失效", 409)
		}
		return err
	})
	return preview, err
}

func (s *Store) GetKnowledgeDocument(
	ctx context.Context, principal domain.Principal, documentID int64,
) (KnowledgeDocument, error) {
	var result KnowledgeDocument
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		return scanKnowledgeDocument(getKnowledgeDocumentTx(ctx, tx, principal, documentID, false), &result)
	})
	return result, err
}

func (s *Store) MarkKnowledgeDocumentDeleting(
	ctx context.Context, principal domain.Principal, documentID int64,
) (KnowledgeDocument, error) {
	var result KnowledgeDocument
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		if err := scanKnowledgeDocument(getKnowledgeDocumentTx(ctx, tx, principal, documentID, true), &result); err != nil {
			return err
		}
		if result.DeletedAt != nil {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE knowledge_documents SET
			status='deleting',deleted_at=now(),updated_at=now(),index_version=index_version+1
			WHERE tenant_id=$1 AND id=$2`, principal.TenantID, documentID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE knowledge_index_jobs SET status='cancelled',
			lease_owner=NULL,lease_until=NULL,updated_at=now()
			WHERE tenant_id=$1 AND document_id=$2 AND status IN ('queued','running')`,
			principal.TenantID, documentID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE knowledge_message_sources SET
			snippet=NULL,parent_id=NULL,child_id=NULL,source_deleted=true
			WHERE tenant_id=$1 AND document_id=$2`,
			principal.TenantID, documentID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM knowledge_parent_chunks
			WHERE tenant_id=$1 AND document_id=$2`, principal.TenantID, documentID); err != nil {
			return err
		}
		return auditResource(ctx, tx, principal, "knowledge.document.delete", "knowledge_document", fmt.Sprint(documentID))
	})
	return result, err
}

func getKnowledgeDocumentTx(
	ctx context.Context, tx pgx.Tx, principal domain.Principal, documentID int64, includeDeleted bool,
) pgx.Row {
	deleted := " AND deleted_at IS NULL"
	lock := ""
	if includeDeleted {
		deleted = ""
		lock = " FOR UPDATE"
	}
	return tx.QueryRow(ctx, `SELECT id,collection_id,original_name,stored_path,mime_type,extension,size,sha256,
		status,page_count,character_count,parent_chunk_count,child_chunk_count,language,
		error_code,error_message,index_version,created_at,updated_at,deleted_at
		FROM knowledge_documents WHERE tenant_id=$1 AND id=$2`+deleted+lock,
		principal.TenantID, documentID)
}

type knowledgeDocumentScanner interface {
	Scan(dest ...any) error
}

func scanKnowledgeDocument(rowScanner knowledgeDocumentScanner, item *KnowledgeDocument) error {
	err := rowScanner.Scan(
		&item.ID, &item.CollectionID, &item.OriginalName, &item.StoredPath, &item.MIMEType,
		&item.Extension, &item.Size, &item.SHA256, &item.Status, &item.PageCount,
		&item.CharacterCount, &item.ParentChunkCount, &item.ChildChunkCount, &item.Language,
		&item.ErrorCode, &item.ErrorMessage, &item.IndexVersion, &item.CreatedAt, &item.UpdatedAt,
		&item.DeletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return apierror.New("DOCUMENT_NOT_FOUND", "知识文件不存在", 404)
	}
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
