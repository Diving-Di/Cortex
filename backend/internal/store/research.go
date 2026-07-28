package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ResearchJob struct {
	ID                 int64           `json:"id"`
	TenantID           uuid.UUID       `json:"-"`
	UserID             int32           `json:"-"`
	Mode               string          `json:"mode"`
	QueryPayload       json.RawMessage `json:"query_payload"`
	TargetCount        int             `json:"target_count"`
	TargetCollectionID *int64          `json:"target_collection_id"`
	Status             string          `json:"status"`
	FoundCount         int             `json:"found_count"`
	CollectedCount     int             `json:"collected_count"`
	OrganizedCount     int             `json:"organized_count"`
	FailedCount        int             `json:"failed_count"`
	SavedCount         int             `json:"saved_count"`
	AttemptCount       int             `json:"attempt_count"`
	MaxAttempts        int             `json:"max_attempts"`
	LastErrorCode      *string         `json:"last_error_code"`
	LastErrorSummary   *string         `json:"last_error_summary"`
	CancelRequestedAt  *time.Time      `json:"cancel_requested_at"`
	StartedAt          *time.Time      `json:"started_at"`
	CompletedAt        *time.Time      `json:"completed_at"`
	Version            int             `json:"version"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type ResearchSource struct {
	ID                 int64           `json:"id"`
	JobID              int64           `json:"job_id"`
	SourceURL          string          `json:"source_url"`
	NormalizedURL      string          `json:"normalized_url"`
	Title              string          `json:"title"`
	AuthorDisplayName  string          `json:"author_display_name"`
	PublishedAt        *time.Time      `json:"published_at"`
	RawContent         string          `json:"raw_content"`
	PublicTags         json.RawMessage `json:"public_tags"`
	ContentHash        *string         `json:"content_hash"`
	Status             string          `json:"status"`
	FailureCode        *string         `json:"failure_code"`
	FailureSummary     *string         `json:"failure_summary"`
	CollectedAt        *time.Time      `json:"collected_at"`
	Version            int             `json:"version"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	Draft              *ResearchDraft  `json:"draft,omitempty"`
	Assets             []ResearchAsset `json:"assets,omitempty"`
	TargetCollectionID *int64          `json:"target_collection_id,omitempty"`
}

type ResearchDraft struct {
	ID                  int64           `json:"id"`
	Summary             string          `json:"summary"`
	KeyPoints           json.RawMessage `json:"key_points"`
	Category            string          `json:"category"`
	SuggestedTags       json.RawMessage `json:"suggested_tags"`
	EditedByUser        bool            `json:"edited_by_user"`
	Status              string          `json:"status"`
	KnowledgeDocumentID *int64          `json:"knowledge_document_id"`
	SourceSnapshotHash  string          `json:"source_snapshot_hash"`
	Version             int             `json:"version"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

type ResearchAsset struct {
	ID         int64     `json:"id"`
	Position   int       `json:"position"`
	StoredPath string    `json:"-"`
	MIMEType   string    `json:"mime_type"`
	ByteSize   int64     `json:"byte_size"`
	SHA256     string    `json:"sha256"`
	OCRStatus  string    `json:"ocr_status"`
	OCRText    string    `json:"ocr_text"`
	CreatedAt  time.Time `json:"created_at"`
}

func (s *Store) CreateResearchJob(
	ctx context.Context, principal domain.Principal, mode string, payload json.RawMessage,
	targetCount int, collectionID *int64, idempotencyKey string, maxAttempts int,
) (ResearchJob, error) {
	var result ResearchJob
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		err := scanResearchJob(tx.QueryRow(ctx, `INSERT INTO research_jobs
			(tenant_id,created_by,mode,query_payload,target_count,target_collection_id,idempotency_key,max_attempts)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT(tenant_id,idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key
			RETURNING id,tenant_id,created_by,mode,query_payload,target_count,target_collection_id,status,
			found_count,collected_count,organized_count,failed_count,saved_count,attempt_count,max_attempts,
			last_error_code,last_error_summary,cancel_requested_at,started_at,completed_at,version,created_at,updated_at`,
			principal.TenantID, principal.UserID, mode, payload, targetCount, collectionID, idempotencyKey, maxAttempts), &result)
		return err
	})
	return result, err
}

func (s *Store) ListResearchJobs(ctx context.Context, principal domain.Principal, limit, offset int) ([]ResearchJob, int64, error) {
	var result []ResearchJob
	var total int64
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM research_jobs WHERE tenant_id=$1`, principal.TenantID).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id,tenant_id,created_by,mode,query_payload,target_count,target_collection_id,status,
			found_count,collected_count,organized_count,failed_count,saved_count,attempt_count,max_attempts,
			last_error_code,last_error_summary,cancel_requested_at,started_at,completed_at,version,created_at,updated_at
			FROM research_jobs WHERE tenant_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3`,
			principal.TenantID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item ResearchJob
			if err := scanResearchJob(rows, &item); err != nil {
				return err
			}
			result = append(result, item)
		}
		return rows.Err()
	})
	return result, total, err
}

func (s *Store) GetResearchJob(ctx context.Context, principal domain.Principal, id int64) (ResearchJob, error) {
	var result ResearchJob
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		return scanResearchJob(tx.QueryRow(ctx, `SELECT id,tenant_id,created_by,mode,query_payload,target_count,target_collection_id,status,
			found_count,collected_count,organized_count,failed_count,saved_count,attempt_count,max_attempts,
			last_error_code,last_error_summary,cancel_requested_at,started_at,completed_at,version,created_at,updated_at
			FROM research_jobs WHERE tenant_id=$1 AND id=$2`, principal.TenantID, id), &result)
	})
	return result, err
}

func (s *Store) CancelResearchJob(ctx context.Context, principal domain.Principal, id int64) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE research_jobs SET cancel_requested_at=now(),
			status=CASE WHEN status='queued' THEN 'cancelled' ELSE status END,
			completed_at=CASE WHEN status='queued' THEN now() ELSE completed_at END,
			version=version+1,updated_at=now()
			WHERE tenant_id=$1 AND id=$2 AND status NOT IN ('completed','failed','cancelled')`,
			principal.TenantID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apierror.New("RESEARCH_JOB_NOT_FOUND", "研究任务不存在或无法取消", 404)
		}
		return nil
	})
}

func (s *Store) RetryResearchJob(ctx context.Context, principal domain.Principal, id int64) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE research_jobs SET status='queued',available_at=now(),
			lease_owner=NULL,lease_until=NULL,last_error_code=NULL,last_error_summary=NULL,
			cancel_requested_at=NULL,completed_at=NULL,version=version+1,updated_at=now()
			WHERE tenant_id=$1 AND id=$2 AND status IN ('failed','cancelled') AND attempt_count<max_attempts`,
			principal.TenantID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apierror.New("RESEARCH_JOB_NOT_RETRYABLE", "研究任务当前无法重试", 409)
		}
		return nil
	})
}

func (s *Store) ClaimResearchJobs(ctx context.Context, owner string, limit int, lease time.Duration) ([]ResearchJob, error) {
	tx, err := s.AdminPool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `WITH candidates AS (
			SELECT id FROM research_jobs
			WHERE ((status='queued' AND available_at<=now()) OR
			       (status IN ('collecting','extracting','organizing') AND lease_until<now()))
			  AND attempt_count<max_attempts AND cancel_requested_at IS NULL
			ORDER BY available_at,id FOR UPDATE SKIP LOCKED LIMIT $1
		)
		UPDATE research_jobs j SET status='collecting',lease_owner=$2,
			lease_until=now()+$3::interval,attempt_count=attempt_count+1,
			started_at=COALESCE(started_at,now()),updated_at=now()
		FROM candidates c WHERE j.id=c.id
		RETURNING j.id,j.tenant_id,j.created_by,j.mode,j.query_payload,j.target_count,j.target_collection_id,j.status,
			j.found_count,j.collected_count,j.organized_count,j.failed_count,j.saved_count,j.attempt_count,j.max_attempts,
			j.last_error_code,j.last_error_summary,j.cancel_requested_at,j.started_at,j.completed_at,j.version,j.created_at,j.updated_at`,
		limit, owner, lease.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ResearchJob
	for rows.Next() {
		var item ResearchJob
		if err := scanResearchJob(rows, &item); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) AddResearchSource(ctx context.Context, principal domain.Principal, jobID int64, sourceURL, normalizedURL string) (ResearchSource, error) {
	var result ResearchSource
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		err := scanResearchSource(tx.QueryRow(ctx, `INSERT INTO research_sources
			(tenant_id,job_id,source_url,normalized_url,status) VALUES($1,$2,$3,$4,'collecting')
			ON CONFLICT(tenant_id,normalized_url) DO UPDATE SET job_id=EXCLUDED.job_id,
				source_url=EXCLUDED.source_url,status=CASE WHEN research_sources.status='saved' THEN 'saved' ELSE 'collecting' END,
				failure_code=NULL,failure_summary=NULL,updated_at=now()
			RETURNING id,job_id,source_url,normalized_url,title,author_display_name,published_at,raw_content,
			public_tags,content_hash,status,failure_code,failure_summary,collected_at,version,created_at,updated_at`,
			principal.TenantID, jobID, sourceURL, normalizedURL), &result)
		return err
	})
	return result, err
}

func (s *Store) CompleteResearchSource(
	ctx context.Context, principal domain.Principal, sourceID int64, title, author, content, contentHash string,
	tags []string, summary string, keyPoints []string, category string, suggestedTags []string, model string,
) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		tagJSON, _ := json.Marshal(tags)
		if _, err := tx.Exec(ctx, `UPDATE research_sources SET title=$3,author_display_name=$4,raw_content=$5,
			public_tags=$6,content_hash=$7,status='pending_review',failure_code=NULL,failure_summary=NULL,
			collected_at=now(),version=version+1,updated_at=now() WHERE tenant_id=$1 AND id=$2`,
			principal.TenantID, sourceID, title, author, content, tagJSON, contentHash); err != nil {
			return err
		}
		var existing *ResearchDraft
		row := tx.QueryRow(ctx, `SELECT id,summary,key_points,category,suggested_tags,edited_by_user,status,
			knowledge_document_id,source_snapshot_hash,version,created_at,updated_at
			FROM research_drafts WHERE tenant_id=$1 AND source_id=$2 FOR UPDATE`, principal.TenantID, sourceID)
		var draft ResearchDraft
		if err := scanResearchDraft(row, &draft); err == nil {
			existing = &draft
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if existing != nil {
			if _, err := tx.Exec(ctx, `INSERT INTO research_draft_revisions
				(tenant_id,draft_id,summary,key_points,category,suggested_tags,reason)
				VALUES($1,$2,$3,$4,$5,$6,'source_recollected')`,
				principal.TenantID, existing.ID, existing.Summary, existing.KeyPoints,
				existing.Category, existing.SuggestedTags); err != nil {
				return err
			}
		}
		pointsJSON, _ := json.Marshal(keyPoints)
		suggestedJSON, _ := json.Marshal(suggestedTags)
		_, err := tx.Exec(ctx, `INSERT INTO research_drafts
			(tenant_id,source_id,summary,key_points,category,suggested_tags,model_name,source_snapshot_hash)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT(tenant_id,source_id) DO UPDATE SET
				summary=CASE WHEN research_drafts.edited_by_user THEN research_drafts.summary ELSE EXCLUDED.summary END,
				key_points=CASE WHEN research_drafts.edited_by_user THEN research_drafts.key_points ELSE EXCLUDED.key_points END,
				category=CASE WHEN research_drafts.edited_by_user THEN research_drafts.category ELSE EXCLUDED.category END,
				suggested_tags=CASE WHEN research_drafts.edited_by_user THEN research_drafts.suggested_tags ELSE EXCLUDED.suggested_tags END,
				model_name=EXCLUDED.model_name,source_snapshot_hash=EXCLUDED.source_snapshot_hash,
				version=research_drafts.version+1,updated_at=now()`,
			principal.TenantID, sourceID, summary, pointsJSON, category, suggestedJSON, model, contentHash)
		return err
	})
}

func (s *Store) FailResearchSource(ctx context.Context, principal domain.Principal, sourceID int64, code, summary string) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE research_sources SET status='failed',failure_code=$3,
			failure_summary=$4,version=version+1,updated_at=now() WHERE tenant_id=$1 AND id=$2`,
			principal.TenantID, sourceID, code, summary)
		return err
	})
}

func (s *Store) CompleteResearchJob(ctx context.Context, principal domain.Principal, jobID int64, failed bool, code string) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		status := "reviewing"
		if failed {
			status = "failed"
		}
		_, err := tx.Exec(ctx, `UPDATE research_jobs j SET status=$3,lease_owner=NULL,lease_until=NULL,
			found_count=(SELECT count(*) FROM research_sources s WHERE s.tenant_id=$1 AND s.job_id=$2 AND s.deleted_at IS NULL),
			collected_count=(SELECT count(*) FROM research_sources s WHERE s.tenant_id=$1 AND s.job_id=$2 AND s.status IN ('pending_review','saved','ignored')),
			organized_count=(SELECT count(*) FROM research_sources s WHERE s.tenant_id=$1 AND s.job_id=$2 AND s.status IN ('pending_review','saved','ignored')),
			failed_count=(SELECT count(*) FROM research_sources s WHERE s.tenant_id=$1 AND s.job_id=$2 AND s.status='failed'),
			last_error_code=NULLIF($4,''),last_error_summary=CASE WHEN $4='' THEN NULL ELSE '研究任务处理失败' END,
			completed_at=now(),version=version+1,updated_at=now()
			WHERE j.tenant_id=$1 AND j.id=$2`, principal.TenantID, jobID, status, code)
		return err
	})
}

func (s *Store) SetResearchJobStage(
	ctx context.Context, principal domain.Principal, jobID int64, status string, lease time.Duration,
) error {
	if status != "collecting" && status != "extracting" && status != "organizing" {
		return fmt.Errorf("invalid research stage")
	}
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE research_jobs SET status=$3,
			lease_until=now()+$4::interval,version=version+1,updated_at=now()
			WHERE tenant_id=$1 AND id=$2 AND cancel_requested_at IS NULL
			AND status IN ('collecting','extracting','organizing')`,
			principal.TenantID, jobID, status, lease.String())
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apierror.New("RESEARCH_JOB_NOT_FOUND", "研究任务已取消或不存在", 404)
		}
		return nil
	})
}

func (s *Store) RequeueResearchJob(
	ctx context.Context, principal domain.Principal, jobID int64, delay time.Duration,
) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE research_jobs SET status='queued',available_at=now()+$3::interval,
			lease_owner=NULL,lease_until=NULL,attempt_count=GREATEST(attempt_count-1,0),updated_at=now()
			WHERE tenant_id=$1 AND id=$2 AND cancel_requested_at IS NULL
			AND status IN ('collecting','extracting','organizing')`,
			principal.TenantID, jobID, delay.String())
		return err
	})
}

type ResearchSourceFilter struct {
	JobID  *int64
	Status string
	Search string
	Sort   string
	Limit  int
	Offset int
}

func (s *Store) ListResearchSources(ctx context.Context, principal domain.Principal, filter ResearchSourceFilter) ([]ResearchSource, int64, error) {
	var result []ResearchSource
	var total int64
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		args := []any{principal.TenantID}
		where := "s.tenant_id=$1 AND s.deleted_at IS NULL"
		if filter.JobID != nil {
			args = append(args, *filter.JobID)
			where += fmt.Sprintf(" AND s.job_id=$%d", len(args))
		}
		if filter.Status != "" {
			args = append(args, filter.Status)
			where += fmt.Sprintf(" AND s.status=$%d", len(args))
		}
		if filter.Search != "" {
			args = append(args, "%"+filter.Search+"%")
			where += fmt.Sprintf(" AND (s.title ILIKE $%d OR s.author_display_name ILIKE $%d)", len(args), len(args))
		}
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM research_sources s WHERE "+where, args...).Scan(&total); err != nil {
			return err
		}
		order := "s.created_at DESC,s.id DESC"
		if filter.Sort == "published_at" {
			order = "s.published_at DESC NULLS LAST,s.id DESC"
		}
		args = append(args, filter.Limit, filter.Offset)
		rows, err := tx.Query(ctx, `SELECT s.id,s.job_id,s.source_url,s.normalized_url,s.title,s.author_display_name,
			s.published_at,s.raw_content,s.public_tags,s.content_hash,s.status,s.failure_code,s.failure_summary,
			s.collected_at,s.version,s.created_at,s.updated_at
			FROM research_sources s WHERE `+where+fmt.Sprintf(" ORDER BY %s LIMIT $%d OFFSET $%d", order, len(args)-1, len(args)), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item ResearchSource
			if err := scanResearchSource(rows, &item); err != nil {
				return err
			}
			result = append(result, item)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()
		for index := range result {
			var draft ResearchDraft
			err := scanResearchDraft(tx.QueryRow(ctx, `SELECT id,summary,key_points,category,suggested_tags,
				edited_by_user,status,knowledge_document_id,source_snapshot_hash,version,created_at,updated_at
				FROM research_drafts WHERE tenant_id=$1 AND source_id=$2`,
				principal.TenantID, result[index].ID), &draft)
			if err == nil {
				result[index].Draft = &draft
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		return nil
	})
	return result, total, err
}

func (s *Store) GetResearchSource(ctx context.Context, principal domain.Principal, id int64) (ResearchSource, error) {
	var result ResearchSource
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		if err := scanResearchSource(tx.QueryRow(ctx, `SELECT id,job_id,source_url,normalized_url,title,
			author_display_name,published_at,raw_content,public_tags,content_hash,status,failure_code,
			failure_summary,collected_at,version,created_at,updated_at
			FROM research_sources WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`,
			principal.TenantID, id), &result); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT target_collection_id FROM research_jobs
			WHERE tenant_id=$1 AND id=$2`, principal.TenantID, result.JobID).Scan(&result.TargetCollectionID); err != nil {
			return err
		}
		var draft ResearchDraft
		err := scanResearchDraft(tx.QueryRow(ctx, `SELECT id,summary,key_points,category,suggested_tags,
			edited_by_user,status,knowledge_document_id,source_snapshot_hash,version,created_at,updated_at
			FROM research_drafts WHERE tenant_id=$1 AND source_id=$2`, principal.TenantID, id), &draft)
		if err == nil {
			result.Draft = &draft
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id,position,storage_path,mime_type,byte_size,sha256,
			ocr_status,ocr_text,created_at FROM research_assets
			WHERE tenant_id=$1 AND source_id=$2 ORDER BY position`, principal.TenantID, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var asset ResearchAsset
			if err := rows.Scan(&asset.ID, &asset.Position, &asset.StoredPath, &asset.MIMEType,
				&asset.ByteSize, &asset.SHA256, &asset.OCRStatus, &asset.OCRText, &asset.CreatedAt); err != nil {
				return err
			}
			result.Assets = append(result.Assets, asset)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) UpdateResearchDraft(
	ctx context.Context, principal domain.Principal, sourceID int64, expectedVersion int,
	summary string, keyPoints, suggestedTags []string, category string,
) (ResearchDraft, error) {
	var result ResearchDraft
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var current ResearchDraft
		if err := scanResearchDraft(tx.QueryRow(ctx, `SELECT id,summary,key_points,category,suggested_tags,
			edited_by_user,status,knowledge_document_id,source_snapshot_hash,version,created_at,updated_at
			FROM research_drafts WHERE tenant_id=$1 AND source_id=$2 FOR UPDATE`,
			principal.TenantID, sourceID), &current); err != nil {
			return err
		}
		if current.Version != expectedVersion {
			return apierror.New("RESEARCH_VERSION_CONFLICT", "研究草稿已被更新，请刷新后重试", 409)
		}
		if current.Status != "pending" {
			return apierror.New("RESEARCH_VERSION_CONFLICT", "研究草稿已完成处理", 409)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO research_draft_revisions
			(tenant_id,draft_id,summary,key_points,category,suggested_tags,reason,created_by)
			VALUES($1,$2,$3,$4,$5,$6,'user_edit',$7)`, principal.TenantID, current.ID,
			current.Summary, current.KeyPoints, current.Category, current.SuggestedTags, principal.UserID); err != nil {
			return err
		}
		pointsJSON, _ := json.Marshal(keyPoints)
		tagsJSON, _ := json.Marshal(suggestedTags)
		return scanResearchDraft(tx.QueryRow(ctx, `UPDATE research_drafts SET summary=$3,key_points=$4,
			category=$5,suggested_tags=$6,edited_by_user=true,version=version+1,updated_at=now()
			WHERE tenant_id=$1 AND source_id=$2
			RETURNING id,summary,key_points,category,suggested_tags,edited_by_user,status,
			knowledge_document_id,source_snapshot_hash,version,created_at,updated_at`,
			principal.TenantID, sourceID, summary, pointsJSON, category, tagsJSON), &result)
	})
	return result, err
}

func (s *Store) IgnoreResearchSources(ctx context.Context, principal domain.Principal, ids []int64) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE research_sources SET status='ignored',version=version+1,updated_at=now()
			WHERE tenant_id=$1 AND id=ANY($2) AND status='pending_review'`, principal.TenantID, ids)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != int64(len(ids)) {
			return apierror.New("RESEARCH_VERSION_CONFLICT", "部分研究结果已发生变化", 409)
		}
		_, err = tx.Exec(ctx, `UPDATE research_drafts SET status='ignored',version=version+1,updated_at=now()
			WHERE tenant_id=$1 AND source_id=ANY($2)`, principal.TenantID, ids)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE research_jobs j SET status='completed',completed_at=now(),
			version=version+1,updated_at=now() WHERE tenant_id=$1
			AND NOT EXISTS(SELECT 1 FROM research_sources s WHERE s.tenant_id=j.tenant_id
				AND s.job_id=j.id AND s.deleted_at IS NULL
				AND s.status IN ('pending','collecting','organizing','pending_review'))`,
			principal.TenantID)
		return err
	})
}

func (s *Store) AddResearchAsset(
	ctx context.Context, principal domain.Principal, sourceID int64, position int,
	storedPath, urlHash, mimeType string, size int64, digest, ocrStatus, ocrText string,
) (ResearchAsset, error) {
	var result ResearchAsset
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO research_assets
			(tenant_id,source_id,position,storage_path,original_url_hash,mime_type,byte_size,sha256,ocr_status,ocr_text)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT(tenant_id,source_id,position) DO UPDATE SET
			storage_path=EXCLUDED.storage_path,original_url_hash=EXCLUDED.original_url_hash,
			mime_type=EXCLUDED.mime_type,byte_size=EXCLUDED.byte_size,sha256=EXCLUDED.sha256,
			ocr_status=EXCLUDED.ocr_status,ocr_text=EXCLUDED.ocr_text,updated_at=now()
			RETURNING id,position,storage_path,mime_type,byte_size,sha256,ocr_status,ocr_text,created_at`,
			principal.TenantID, sourceID, position, storedPath, urlHash, mimeType, size,
			digest, ocrStatus, ocrText).Scan(&result.ID, &result.Position, &result.StoredPath,
			&result.MIMEType, &result.ByteSize, &result.SHA256, &result.OCRStatus,
			&result.OCRText, &result.CreatedAt)
	})
	return result, err
}

func (s *Store) GetResearchAsset(
	ctx context.Context, principal domain.Principal, id int64,
) (ResearchAsset, error) {
	var result ResearchAsset
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT a.id,a.position,a.storage_path,a.mime_type,a.byte_size,a.sha256,
			a.ocr_status,a.ocr_text,a.created_at FROM research_assets a
			JOIN research_sources s ON s.tenant_id=a.tenant_id AND s.id=a.source_id
			WHERE a.tenant_id=$1 AND a.id=$2 AND s.deleted_at IS NULL`,
			principal.TenantID, id).Scan(&result.ID, &result.Position, &result.StoredPath,
			&result.MIMEType, &result.ByteSize, &result.SHA256, &result.OCRStatus,
			&result.OCRText, &result.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("RESEARCH_SOURCE_NOT_FOUND", "研究图片不存在", 404)
		}
		return err
	})
	return result, err
}

func (s *Store) MarkResearchSourceSaved(
	ctx context.Context, principal domain.Principal, sourceID, documentID int64,
) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var jobID int64
		err := tx.QueryRow(ctx, `UPDATE research_sources SET status='saved',version=version+1,updated_at=now()
			WHERE tenant_id=$1 AND id=$2 AND status IN ('pending_review','saved') RETURNING job_id`,
			principal.TenantID, sourceID).Scan(&jobID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("RESEARCH_VERSION_CONFLICT", "研究结果当前无法保存", 409)
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE research_drafts SET status='saved',
			knowledge_document_id=COALESCE(knowledge_document_id,$3),version=version+1,updated_at=now()
			WHERE tenant_id=$1 AND source_id=$2`, principal.TenantID, sourceID, documentID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE research_jobs SET saved_count=(
			SELECT count(*) FROM research_sources WHERE tenant_id=$1 AND job_id=$2 AND status='saved'
			),status=CASE WHEN NOT EXISTS(
				SELECT 1 FROM research_sources WHERE tenant_id=$1 AND job_id=$2
				AND status IN ('pending','collecting','organizing','pending_review')
			) THEN 'completed' ELSE status END,version=version+1,updated_at=now()
			WHERE tenant_id=$1 AND id=$2`, principal.TenantID, jobID)
		return err
	})
}

func (s *Store) SoftDeleteResearchSource(
	ctx context.Context, principal domain.Principal, sourceID int64,
) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE research_sources SET deleted_at=now(),
			version=version+1,updated_at=now() WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`,
			principal.TenantID, sourceID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apierror.New("RESEARCH_SOURCE_NOT_FOUND", "研究结果不存在", 404)
		}
		return auditResource(ctx, tx, principal, "research.source.delete", "research_source", fmt.Sprint(sourceID))
	})
}

func scanResearchJob(scanner knowledgeDocumentScanner, item *ResearchJob) error {
	err := scanner.Scan(&item.ID, &item.TenantID, &item.UserID, &item.Mode, &item.QueryPayload,
		&item.TargetCount, &item.TargetCollectionID, &item.Status, &item.FoundCount,
		&item.CollectedCount, &item.OrganizedCount, &item.FailedCount, &item.SavedCount,
		&item.AttemptCount, &item.MaxAttempts, &item.LastErrorCode, &item.LastErrorSummary,
		&item.CancelRequestedAt, &item.StartedAt, &item.CompletedAt, &item.Version,
		&item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return apierror.New("RESEARCH_JOB_NOT_FOUND", "研究任务不存在", 404)
	}
	return err
}

func scanResearchSource(scanner knowledgeDocumentScanner, item *ResearchSource) error {
	err := scanner.Scan(&item.ID, &item.JobID, &item.SourceURL, &item.NormalizedURL, &item.Title,
		&item.AuthorDisplayName, &item.PublishedAt, &item.RawContent, &item.PublicTags,
		&item.ContentHash, &item.Status, &item.FailureCode, &item.FailureSummary,
		&item.CollectedAt, &item.Version, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return apierror.New("RESEARCH_SOURCE_NOT_FOUND", "研究结果不存在", 404)
	}
	return err
}

func scanResearchDraft(scanner knowledgeDocumentScanner, item *ResearchDraft) error {
	return scanner.Scan(&item.ID, &item.Summary, &item.KeyPoints, &item.Category,
		&item.SuggestedTags, &item.EditedByUser, &item.Status, &item.KnowledgeDocumentID,
		&item.SourceSnapshotHash, &item.Version, &item.CreatedAt, &item.UpdatedAt)
}

func ResearchTextFile(source ResearchSource) string {
	var points []string
	if source.Draft != nil {
		_ = json.Unmarshal(source.Draft.KeyPoints, &points)
	}
	var builder strings.Builder
	builder.WriteString("# " + source.Title + "\n\n")
	if source.Draft != nil {
		builder.WriteString("## 摘要\n\n" + source.Draft.Summary + "\n\n")
		if len(points) > 0 {
			builder.WriteString("## 关键观点\n\n")
			for _, point := range points {
				builder.WriteString("- " + point + "\n")
			}
			builder.WriteString("\n")
		}
	}
	builder.WriteString("## 来源原文\n\n" + source.RawContent + "\n\n")
	builder.WriteString("来源：" + source.SourceURL + "\n")
	return builder.String()
}
