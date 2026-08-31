package store

import (
	"context"
	"errors"
	"time"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
	"cortex/backend/internal/knowledge"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const KnowledgeQuotaBytes int64 = 3221225472

type KnowledgeDocument struct {
	ID                        uuid.UUID  `json:"id"`
	UploadID                  *uuid.UUID `json:"upload_id,omitempty"`
	CollectionID              *uuid.UUID `json:"collection_id,omitempty"`
	SourceType, Title, Status string
	StoredPath                *string `json:"-"`
	SizeBytes                 int64   `json:"size_bytes"`
	ActiveIndexVersion        int     `json:"active_index_version"`
	FailureCode               *string `json:"failure_code,omitempty"`
	FailureSummary            *string `json:"failure_summary,omitempty"`
	LastIndexFailureCode      *string `json:"last_index_failure_code,omitempty"`
	IndexJobStatus            *string `json:"index_job_status,omitempty"`
	IndexStage                *string `json:"index_stage,omitempty"`
	ProcessedChunks           int     `json:"processed_chunks"`
	TotalChunks               int     `json:"total_chunks"`
	CreatedAt, UpdatedAt      time.Time
}
type KnowledgeUpload struct {
	ID                   uuid.UUID `json:"id"`
	OriginalName, Status string
	ExpandedBytes        int64   `json:"expanded_bytes"`
	FailureCode          *string `json:"failure_code,omitempty"`
	FailureSummary       *string `json:"failure_summary,omitempty"`
	CreatedAt, UpdatedAt time.Time
}
type KnowledgeCollection struct {
	ID                   uuid.UUID `json:"id"`
	Name, Description    string
	CreatedAt, UpdatedAt time.Time
}

type KnowledgeAsset struct {
	ID                               uuid.UUID
	StoredPath, StorageBackend, MIME string
	SizeBytes                        int64
}

func (s *Store) GetKnowledgeAsset(ctx context.Context, p domain.Principal, documentID, assetID uuid.UUID) (KnowledgeAsset, error) {
	var a KnowledgeAsset
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT a.id,coalesce(a.object_key,a.stored_path),a.storage_backend,a.mime_type,a.size_bytes FROM knowledge_assets a JOIN knowledge_documents d ON d.tenant_id=a.tenant_id AND d.id=a.document_id WHERE a.tenant_id=$1 AND a.document_id=$2 AND a.id=$3 AND d.deleted_at IS NULL`, p.TenantID, documentID, assetID).Scan(&a.ID, &a.StoredPath, &a.StorageBackend, &a.MIME, &a.SizeBytes)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("KNOWLEDGE_SCOPE_NOT_FOUND", "知识库资源不存在", 404)
		}
		return err
	})
	return a, err
}
func (s *Store) RetryKnowledgeDocument(ctx context.Context, p domain.Principal, id uuid.UUID) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		var version int
		if err := tx.QueryRow(ctx, `UPDATE knowledge_documents SET status=CASE WHEN active_index_version=0 THEN 'indexing' ELSE 'ready' END,failure_code=CASE WHEN active_index_version=0 THEN NULL ELSE failure_code END,failure_summary=CASE WHEN active_index_version=0 THEN NULL ELSE failure_summary END,updated_at=now() WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL AND knowledge_enabled RETURNING active_index_version+1`, p.TenantID, id).Scan(&version); errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("KNOWLEDGE_SCOPE_NOT_FOUND", "知识库资源不存在", 404)
		} else if err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO knowledge_index_jobs(tenant_id,document_id,target_index_version) VALUES($1,$2,$3) ON CONFLICT(tenant_id,document_id,target_index_version) DO UPDATE SET status='queued',stage='queued',processed_chunks=0,total_chunks=0,available_at=now(),failure_code=NULL,updated_at=now()`, p.TenantID, id, version)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,topic,partition_key,schema_version) VALUES($1,'knowledge',$2::text,'knowledge.index.requested','cortex.knowledge.index.v1',$2::text,1)`, uuid.New(), id.String())
		return err
	})
}
func (s *Store) ListKnowledgeCollections(ctx context.Context, p domain.Principal) ([]KnowledgeCollection, error) {
	var out []KnowledgeCollection
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id,name,description,created_at,updated_at FROM knowledge_collections WHERE tenant_id=$1 AND deleted_at IS NULL ORDER BY name`, p.TenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c KnowledgeCollection
			if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.CreatedAt, &c.UpdatedAt); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}
func (s *Store) CreateKnowledgeCollection(ctx context.Context, p domain.Principal, name, description string) (KnowledgeCollection, error) {
	var c KnowledgeCollection
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO knowledge_collections(tenant_id,name,description) VALUES($1,$2,$3) RETURNING id,name,description,created_at,updated_at`, p.TenantID, name, description).Scan(&c.ID, &c.Name, &c.Description, &c.CreatedAt, &c.UpdatedAt)
	})
	return c, err
}

func (s *Store) CreateKnowledgeUpload(ctx context.Context, p domain.Principal, uploadID uuid.UUID, idempotencyKey, originalName, storedRoot, storageBackend string, prepared knowledge.Prepared) (KnowledgeUpload, error) {
	var result KnowledgeUpload
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		if idempotencyKey != "" {
			err := tx.QueryRow(ctx, `SELECT id,original_name,status,expanded_bytes,failure_code,failure_summary,created_at,updated_at FROM knowledge_uploads WHERE tenant_id=$1 AND idempotency_key=$2`, p.TenantID, idempotencyKey).Scan(&result.ID, &result.OriginalName, &result.Status, &result.ExpandedBytes, &result.FailureCode, &result.FailureSummary, &result.CreatedAt, &result.UpdatedAt)
			if err == nil {
				return nil
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO knowledge_quotas(tenant_id) VALUES($1) ON CONFLICT DO NOTHING`, p.TenantID); err != nil {
			return err
		}
		var used, reserved int64
		if err := tx.QueryRow(ctx, `SELECT used_bytes,reserved_bytes FROM knowledge_quotas WHERE tenant_id=$1 FOR UPDATE`, p.TenantID).Scan(&used, &reserved); err != nil {
			return err
		}
		if prepared.ExpandedBytes > KnowledgeQuotaBytes-used-reserved {
			return apierror.New("KNOWLEDGE_QUOTA_EXCEEDED", "知识库容量已达到上限", 409)
		}
		if _, err := tx.Exec(ctx, `UPDATE knowledge_quotas SET used_bytes=used_bytes+$2,updated_at=now() WHERE tenant_id=$1`, p.TenantID, prepared.ExpandedBytes); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO knowledge_uploads(id,tenant_id,idempotency_key,original_name,stored_root,expanded_bytes,status) VALUES($1,$2,nullif($3,''),$4,$5,$6,'indexing') RETURNING id,original_name,status,expanded_bytes,failure_code,failure_summary,created_at,updated_at`, uploadID, p.TenantID, idempotencyKey, originalName, storedRoot, prepared.ExpandedBytes).Scan(&result.ID, &result.OriginalName, &result.Status, &result.ExpandedBytes, &result.FailureCode, &result.FailureSummary, &result.CreatedAt, &result.UpdatedAt); err != nil {
			return err
		}
		for _, d := range prepared.Documents {
			var documentID uuid.UUID
			stored := storedRoot + "/" + d.RelativePath
			if err := tx.QueryRow(ctx, `INSERT INTO knowledge_documents(tenant_id,upload_id,source_type,title,stored_path,storage_backend,object_key,source_encoding,size_bytes,content_hash,status) VALUES($1,$2,'upload',$3,$4,$5,$4,$6,$7,$8,'indexing') RETURNING id`, p.TenantID, uploadID, d.Title, stored, storageBackend, d.Encoding, d.Size, d.Hash).Scan(&documentID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO knowledge_index_jobs(tenant_id,document_id,target_index_version) VALUES($1,$2,1)`, p.TenantID, documentID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,topic,partition_key,schema_version) VALUES($1,'knowledge',$2::text,'knowledge.index.requested','cortex.knowledge.index.v1',$2::text,1)`, uuid.New(), documentID.String()); err != nil {
				return err
			}
		}
		// Assets are upload-scoped on disk. Associate each with the first Markdown document; access still verifies tenant and upload root.
		if len(prepared.Documents) > 0 && len(prepared.Assets) > 0 {
			var first uuid.UUID
			if err := tx.QueryRow(ctx, `SELECT id FROM knowledge_documents WHERE tenant_id=$1 AND upload_id=$2 ORDER BY created_at,id LIMIT 1`, p.TenantID, uploadID).Scan(&first); err != nil {
				return err
			}
			for _, a := range prepared.Assets {
				stored := storedRoot + "/" + a.RelativePath
				if _, err := tx.Exec(ctx, `INSERT INTO knowledge_assets(tenant_id,document_id,stored_path,storage_backend,object_key,mime_type,size_bytes,sha256) VALUES($1,$2,$3,$4,$3,$5,$6,$7)`, p.TenantID, first, stored, storageBackend, a.MIME, a.Size, a.Hash); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return result, err
}

func (s *Store) GetKnowledgeUpload(ctx context.Context, p domain.Principal, id uuid.UUID) (KnowledgeUpload, error) {
	var v KnowledgeUpload
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT id,original_name,status,expanded_bytes,failure_code,failure_summary,created_at,updated_at FROM knowledge_uploads WHERE tenant_id=$1 AND id=$2`, p.TenantID, id).Scan(&v.ID, &v.OriginalName, &v.Status, &v.ExpandedBytes, &v.FailureCode, &v.FailureSummary, &v.CreatedAt, &v.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("KNOWLEDGE_SCOPE_NOT_FOUND", "知识库资源不存在", 404)
		}
		return err
	})
	return v, err
}
func (s *Store) ListKnowledgeDocuments(ctx context.Context, p domain.Principal) ([]KnowledgeDocument, int64, int64, error) {
	var out []KnowledgeDocument
	var used, reserved int64
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		_ = tx.QueryRow(ctx, `SELECT used_bytes,reserved_bytes FROM knowledge_quotas WHERE tenant_id=$1`, p.TenantID).Scan(&used, &reserved)
		rows, err := tx.Query(ctx, `SELECT d.id,d.upload_id,d.collection_id,d.source_type,d.title,d.status,d.stored_path,d.size_bytes,d.active_index_version,d.failure_code,d.failure_summary,d.last_index_failure_code,j.status,j.stage,coalesce(j.processed_chunks,0),coalesce(j.total_chunks,0),d.created_at,d.updated_at FROM knowledge_documents d LEFT JOIN LATERAL (SELECT status,stage,processed_chunks,total_chunks FROM knowledge_index_jobs WHERE tenant_id=d.tenant_id AND document_id=d.id ORDER BY target_index_version DESC,id DESC LIMIT 1) j ON true WHERE d.tenant_id=$1 AND d.deleted_at IS NULL ORDER BY d.updated_at DESC`, p.TenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v KnowledgeDocument
			if err := rows.Scan(&v.ID, &v.UploadID, &v.CollectionID, &v.SourceType, &v.Title, &v.Status, &v.StoredPath, &v.SizeBytes, &v.ActiveIndexVersion, &v.FailureCode, &v.FailureSummary, &v.LastIndexFailureCode, &v.IndexJobStatus, &v.IndexStage, &v.ProcessedChunks, &v.TotalChunks, &v.CreatedAt, &v.UpdatedAt); err != nil {
				return err
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, used, reserved, err
}
func (s *Store) DeleteKnowledgeDocument(ctx context.Context, p domain.Principal, id uuid.UUID) (string, int64, error) {
	var stored string
	var size int64
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		var uploadID *uuid.UUID
		var backend, objectVersion string
		err := tx.QueryRow(ctx, `UPDATE knowledge_documents SET status='deleting',deleted_at=now(),updated_at=now() WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL RETURNING coalesce(object_key,stored_path,''),storage_backend,coalesce(object_version,''),size_bytes,upload_id`, p.TenantID, id).Scan(&stored, &backend, &objectVersion, &size, &uploadID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("KNOWLEDGE_SCOPE_NOT_FOUND", "知识库资源不存在", 404)
		}
		if err != nil {
			return err
		}
		if stored != "" {
			if _, err = tx.Exec(ctx, `INSERT INTO object_gc_jobs(tenant_id,storage_backend,object_key,object_version) VALUES($1,$2,$3,nullif($4,'')) ON CONFLICT DO NOTHING`, p.TenantID, backend, stored, objectVersion); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO object_gc_jobs(tenant_id,storage_backend,object_key,object_version) SELECT tenant_id,storage_backend,coalesce(object_key,stored_path),object_version FROM knowledge_assets WHERE tenant_id=$1 AND document_id=$2 ON CONFLICT DO NOTHING`, p.TenantID, id)
		return err
	})
	return stored, size, err
}
func (s *Store) FinalizeKnowledgeDeletion(ctx context.Context, p domain.Principal, id uuid.UUID, size int64) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM knowledge_documents WHERE tenant_id=$1 AND id=$2 AND status='deleting'`, p.TenantID, id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE knowledge_quotas SET used_bytes=greatest(0,used_bytes-$2),updated_at=now() WHERE tenant_id=$1`, p.TenantID, size)
		return err
	})
}

func (s *Store) SetNoteKnowledge(ctx context.Context, p domain.Principal, noteID int32, enabled bool) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		var title, content string
		var deleted *time.Time
		err := tx.QueryRow(ctx, `SELECT title,content,deleted_at FROM notes WHERE tenant_id=$1 AND id=$2`, p.TenantID, noteID).Scan(&title, &content, &deleted)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("NOTE_NOT_FOUND", "笔记不存在", 404)
		} else if err != nil {
			return err
		}
		if !enabled || deleted != nil {
			_, err := tx.Exec(ctx, `UPDATE knowledge_documents SET knowledge_enabled=false,status='deleting',updated_at=now() WHERE tenant_id=$1 AND note_id=$2`, p.TenantID, noteID)
			return err
		}
		var id uuid.UUID
		err = tx.QueryRow(ctx, `INSERT INTO knowledge_documents(tenant_id,source_type,note_id,title,content_hash,status,knowledge_enabled) VALUES($1,'note',$2,$3,encode(digest(convert_to($4,'UTF8'),'sha256'),'hex'),'indexing',true) ON CONFLICT (tenant_id,note_id) WHERE source_type='note' DO UPDATE SET title=excluded.title,content_hash=excluded.content_hash,status=CASE WHEN knowledge_documents.active_index_version=0 THEN 'indexing' ELSE 'ready' END,knowledge_enabled=true,deleted_at=NULL,updated_at=now() RETURNING id`, p.TenantID, noteID, title, content).Scan(&id)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO knowledge_index_jobs(tenant_id,document_id,target_index_version) SELECT $1,$2,active_index_version+1 FROM knowledge_documents WHERE tenant_id=$1 AND id=$2 ON CONFLICT(tenant_id,document_id,target_index_version) DO UPDATE SET status='queued',stage='queued',processed_chunks=0,total_chunks=0,available_at=now(),failure_code=NULL,lease_owner=NULL,lease_until=NULL,updated_at=now()`, p.TenantID, id)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,topic,partition_key,schema_version) VALUES($1,'knowledge',$2::text,'knowledge.index.requested','cortex.knowledge.index.v1',$2::text,1)`, uuid.New(), id.String())
		return err
	})
}
