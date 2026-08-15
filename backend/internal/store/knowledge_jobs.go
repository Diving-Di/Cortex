package store

import (
	"context"
	"errors"
	"time"

	"cortex/backend/internal/domain"
	"cortex/backend/internal/knowledge"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type KnowledgeIndexJob struct {
	ID                                     int64
	TenantID, DocumentID                   uuid.UUID
	TargetVersion                          int
	LeaseOwner                             uuid.UUID
	Title, SourceType, StoredPath, Content string
}

var ErrKnowledgeIndexLeaseLost = errors.New("knowledge index lease lost")

func (s *Store) ClaimKnowledgeJobs(ctx context.Context, owner uuid.UUID, limit int, lease time.Duration) ([]KnowledgeIndexJob, error) {
	tx, err := s.AdminPool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `WITH c AS (SELECT id FROM knowledge_index_jobs WHERE (status='queued' AND available_at<=now()) OR (status='running' AND lease_until<now()) ORDER BY available_at,id FOR UPDATE SKIP LOCKED LIMIT $1) UPDATE knowledge_index_jobs j SET status='running',lease_owner=$2,lease_until=now()+$3::interval,attempts=attempts+1,updated_at=now() FROM c WHERE j.id=c.id RETURNING j.id,j.tenant_id,j.document_id,j.target_index_version,j.lease_owner`, limit, owner, lease.String())
	if err != nil {
		return nil, err
	}
	var jobs []KnowledgeIndexJob
	for rows.Next() {
		var j KnowledgeIndexJob
		if err := rows.Scan(&j.ID, &j.TenantID, &j.DocumentID, &j.TargetVersion, &j.LeaseOwner); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return jobs, nil
}
func (s *Store) LoadKnowledgeJobDocument(ctx context.Context, j *KnowledgeIndexJob) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		p := domainPrincipal(j.TenantID)
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT d.title,d.source_type,coalesce(d.stored_path,''),coalesce(n.content,'') FROM knowledge_documents d LEFT JOIN notes n ON n.tenant_id=d.tenant_id AND n.id=d.note_id WHERE d.tenant_id=$1 AND d.id=$2 AND d.deleted_at IS NULL AND d.knowledge_enabled`, j.TenantID, j.DocumentID).Scan(&j.Title, &j.SourceType, &j.StoredPath, &j.Content)
	})
}
func domainPrincipal(tenant uuid.UUID) domain.Principal {
	return domain.Principal{TenantID: tenant, TenantActive: true}
}
func (s *Store) WriteKnowledgeChunks(ctx context.Context, j KnowledgeIndexJob, parents []knowledge.ParentChunk, vectors [][][]float32, model string) error {
	return s.writeKnowledgeChunks(ctx, j, parents, vectors, model, nil)
}

func (s *Store) writeKnowledgeChunks(ctx context.Context, j KnowledgeIndexJob, parents []knowledge.ParentChunk, vectors [][][]float32, model string, beforeActivate func() error) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, domainPrincipal(j.TenantID)); err != nil {
			return err
		}
		var fencedID int64
		if err := tx.QueryRow(ctx, `UPDATE knowledge_index_jobs SET status='success',lease_owner=NULL,lease_until=NULL,updated_at=now() WHERE tenant_id=$1 AND id=$2 AND status='running' AND lease_owner=$3 AND lease_until>now() RETURNING id`, j.TenantID, j.ID, j.LeaseOwner).Scan(&fencedID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrKnowledgeIndexLeaseLost
			}
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM knowledge_parent_chunks WHERE tenant_id=$1 AND document_id=$2 AND index_version=$3`, j.TenantID, j.DocumentID, j.TargetVersion); err != nil {
			return err
		}
		for pi, p := range parents {
			heading := p.Heading
			if heading == nil {
				heading = []string{}
			}
			var parentID uuid.UUID
			if err := tx.QueryRow(ctx, `INSERT INTO knowledge_parent_chunks(tenant_id,document_id,index_version,ordinal,heading_path,content,content_hash) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, j.TenantID, j.DocumentID, j.TargetVersion, pi, heading, p.Content, p.Hash).Scan(&parentID); err != nil {
				return err
			}
			for ci, c := range p.Children {
				if _, err := tx.Exec(ctx, `INSERT INTO knowledge_child_chunks(tenant_id,parent_id,document_id,index_version,ordinal,content,embedding_text,keyword_text,embedding,embedding_model,content_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::vector,$10,$11)`, j.TenantID, parentID, j.DocumentID, j.TargetVersion, ci, c.Content, c.EmbeddingText, c.KeywordText, vectorLiteral(vectors[pi][ci]), model, c.Hash); err != nil {
					return err
				}
			}
		}
		if beforeActivate != nil {
			if err := beforeActivate(); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE knowledge_documents SET active_index_version=$3,status='ready',failure_code=NULL,failure_summary=NULL,last_index_failure_code=NULL,updated_at=now() WHERE tenant_id=$1 AND id=$2`, j.TenantID, j.DocumentID, j.TargetVersion); err != nil {
			return err
		}
		// Keep the active version and its immediate predecessor. Version N-2 is
		// removed only after N succeeds, leaving one full rebuild cycle for rollback.
		// Historical message sources retain their title, snippet, and index_version.
		if _, err := tx.Exec(ctx, `DELETE FROM knowledge_parent_chunks WHERE tenant_id=$1 AND document_id=$2 AND index_version < $3 - 1`, j.TenantID, j.DocumentID, j.TargetVersion); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE knowledge_uploads u SET status='ready',updated_at=now() WHERE u.tenant_id=$1 AND NOT EXISTS(SELECT 1 FROM knowledge_documents d WHERE d.tenant_id=u.tenant_id AND d.upload_id=u.id AND d.status<>'ready')`, j.TenantID)
		return err
	})
}
func (s *Store) FailKnowledgeJob(ctx context.Context, j KnowledgeIndexJob, code string) error {
	tx, err := s.AdminPool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var jobStatus string
	if err := tx.QueryRow(ctx, `UPDATE knowledge_index_jobs SET status=CASE WHEN attempts<3 THEN 'queued' ELSE 'failed' END,available_at=now()+make_interval(secs=>least(3600,attempts*attempts*10)),failure_code=$2,lease_owner=NULL,lease_until=NULL,updated_at=now() WHERE id=$1 AND tenant_id=$3 AND status='running' AND lease_owner=$4 AND lease_until>now() RETURNING status`, j.ID, code, j.TenantID, j.LeaseOwner).Scan(&jobStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrKnowledgeIndexLeaseLost
		}
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE knowledge_documents SET status=CASE WHEN active_index_version>0 THEN 'ready' WHEN $4='failed' THEN 'failed' ELSE 'indexing' END,failure_code=CASE WHEN active_index_version=0 AND $4='failed' THEN $3 ELSE NULL END,failure_summary=CASE WHEN active_index_version=0 AND $4='failed' THEN '索引服务暂时不可用' ELSE NULL END,last_index_failure_code=CASE WHEN active_index_version>0 THEN $3 ELSE last_index_failure_code END,updated_at=now() WHERE tenant_id=$1 AND id=$2`, j.TenantID, j.DocumentID, code, jobStatus); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
