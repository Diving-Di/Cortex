package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrReportJobLeaseLost = errors.New("report job lease lost")

func (s *Store) ValidateReportJobSources(ctx context.Context, p domain.Principal, id uuid.UUID) error {
	return s.WithPrincipalTx(ctx, p, func(tx pgx.Tx) error {
		var valid bool
		err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tenants WHERE id=$1 AND deleted_at IS NULL)
  AND j.source_versions=(SELECT COALESCE(jsonb_object_agg(n.id::text,n.updated_at::text),'{}') FROM notes n WHERE n.tenant_id=$1 AND n.deleted_at IS NULL AND j.source_versions ? n.id::text)
  FROM report_generation_jobs j WHERE j.tenant_id=$1 AND j.id=$2`, p.TenantID, id).Scan(&valid)
		if err != nil {
			return err
		}
		if !valid {
			return apierror.New("REPORT_SOURCES_CHANGED", "来源已修改或删除，请重新生成", 409)
		}
		return nil
	})
}

type ReportJob struct {
	ID              uuid.UUID    `json:"id"`
	RequestID       uuid.UUID    `json:"request_id"`
	Type            string       `json:"type"`
	AnchorDate      string       `json:"anchor_date"`
	Status          string       `json:"status"`
	Stage           string       `json:"stage"`
	Sources         []SourceNote `json:"sources"`
	Content         string       `json:"content"`
	FailureCode     *string      `json:"failure_code"`
	ConfirmedNoteID *int32       `json:"confirmed_note_id"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

const reportJobColumns = `id,request_id,report_type,anchor_date::text,status,stage,sources,content,failure_code,confirmed_note_id,created_at,updated_at`

func scanReportJob(row interface{ Scan(...any) error }) (ReportJob, error) {
	var j ReportJob
	var raw []byte
	err := row.Scan(&j.ID, &j.RequestID, &j.Type, &j.AnchorDate, &j.Status, &j.Stage, &raw, &j.Content, &j.FailureCode, &j.ConfirmedNoteID, &j.CreatedAt, &j.UpdatedAt)
	if err == nil {
		err = json.Unmarshal(raw, &j.Sources)
	}
	return j, err
}

func reportJobEvent(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	_, err := tx.Exec(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,topic,partition_key,schema_version) VALUES($1,'report',$2::text,'report.generate.requested','cortex.report.generate.v1',$2::text,1)`, uuid.New(), id.String())
	return err
}

func (s *Store) CreateReportJob(ctx context.Context, p domain.Principal, requestID uuid.UUID, kind string, anchor time.Time) (ReportJob, error) {
	var j ReportJob
	err := s.WithPrincipalTx(ctx, p, func(tx pgx.Tx) error {
		var err error
		j, err = createReportJobTx(ctx, tx, p, requestID, kind, anchor)
		return err
	})
	return j, err
}

func createReportJobTx(ctx context.Context, tx pgx.Tx, p domain.Principal, requestID uuid.UUID, kind string, anchor time.Time) (ReportJob, error) {
	start, end := periodRange(kind, anchor)
	// Serialize admission and idempotency for this tenant across all API instances.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,43))`, p.TenantID.String()); err != nil {
		return ReportJob{}, err
	}
	existing, err := scanReportJob(tx.QueryRow(ctx, `SELECT `+reportJobColumns+` FROM report_generation_jobs WHERE tenant_id=$1 AND request_id=$2`, p.TenantID, requestID))
	if err == nil {
		if existing.Type != kind || existing.AnchorDate != start.Format(time.DateOnly) {
			return ReportJob{}, apierror.New("IDEMPOTENCY_CONFLICT", "相同请求标识已用于其他周期", 409)
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ReportJob{}, err
	}
	var active int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM report_generation_jobs WHERE tenant_id=$1 AND status IN ('queued','running')`, p.TenantID).Scan(&active); err != nil {
		return ReportJob{}, err
	}
	if active >= 3 {
		return ReportJob{}, apierror.New("REPORT_QUEUE_FULL", "最多同时提交三个后台报告，请等待现有任务完成", 429)
	}
	sources, err := reportSourcesTx(ctx, tx, p, kind, start, end)
	if err != nil {
		return ReportJob{}, err
	}
	if len(sources) == 0 {
		return ReportJob{}, apierror.New("REPORT_NO_SOURCES", "所选周期没有来源笔记", 422)
	}
	ids := make([]int32, 0, len(sources))
	for _, source := range sources {
		ids = append(ids, source.ID)
	}
	raw, err := json.Marshal(sources)
	if err != nil {
		return ReportJob{}, err
	}
	id := uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO report_generation_jobs(id,tenant_id,created_by,request_id,report_type,anchor_date,sources,source_versions,target_updated_at,dispatch_at)
 VALUES($1,$2,$3,$4,$5::varchar,$6::date,$7,
 (SELECT jsonb_object_agg(id::text,updated_at::text) FROM notes WHERE tenant_id=$2 AND id=ANY($8)),
 (SELECT updated_at FROM notes WHERE tenant_id=$2 AND type=$5 AND note_date=$6 AND deleted_at IS NULL),now()+interval '1 minute')`, id, p.TenantID, p.UserID, requestID, kind, start, raw, ids)
	if err != nil {
		return ReportJob{}, err
	}
	if err = reportJobEvent(ctx, tx, id); err != nil {
		return ReportJob{}, err
	}
	return scanReportJob(tx.QueryRow(ctx, `SELECT `+reportJobColumns+` FROM report_generation_jobs WHERE tenant_id=$1 AND id=$2`, p.TenantID, id))
}

func (s *Store) GetReportJob(ctx context.Context, p domain.Principal, id uuid.UUID) (ReportJob, error) {
	var j ReportJob
	err := s.WithPrincipalTx(ctx, p, func(tx pgx.Tx) error {
		var err error
		j, err = scanReportJob(tx.QueryRow(ctx, `SELECT `+reportJobColumns+` FROM report_generation_jobs WHERE tenant_id=$1 AND id=$2`, p.TenantID, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("REPORT_JOB_NOT_FOUND", "报告任务不存在", 404)
		}
		return err
	})
	return j, err
}

func (s *Store) ListReportJobs(ctx context.Context, p domain.Principal) ([]ReportJob, error) {
	result := []ReportJob{}
	err := s.WithPrincipalTx(ctx, p, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+reportJobColumns+` FROM report_generation_jobs WHERE tenant_id=$1 ORDER BY created_at DESC,id LIMIT 50`, p.TenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			j, err := scanReportJob(rows)
			if err != nil {
				return err
			}
			j.Content = ""
			for i := range j.Sources {
				j.Sources[i].Snippet = ""
			}
			result = append(result, j)
		}
		return rows.Err()
	})
	return result, err
}

// Claim uses the management pool only to select trusted identity and acquire a
// bounded lease. Business material is subsequently read under tenant RLS.
func (s *Store) ClaimReportJob(ctx context.Context, id, owner uuid.UUID) (domain.Principal, bool, error) {
	var p domain.Principal
	tx, err := s.AdminPool.Begin(ctx)
	if err != nil {
		return p, false, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(430043)`); err != nil {
		return p, false, err
	}
	var running int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM report_generation_jobs WHERE status='running' AND lease_until>now()`).Scan(&running); err != nil {
		return p, false, err
	}
	if running >= 4 {
		return p, false, nil
	}
	err = tx.QueryRow(ctx, `WITH candidate AS (SELECT id FROM report_generation_jobs WHERE id=$1 AND status='queued' FOR UPDATE SKIP LOCKED)
 UPDATE report_generation_jobs j SET status='running',stage='generating',lease_owner=$2,lease_until=now()+interval '90 seconds',updated_at=now()
 FROM tenants t WHERE j.id=$1 AND j.status='queued' AND t.id=j.tenant_id AND t.deleted_at IS NULL
 AND j.id IN (SELECT id FROM candidate)
 AND NOT EXISTS(SELECT 1 FROM report_generation_jobs other WHERE other.tenant_id=j.tenant_id AND other.status='running' AND other.lease_until>now())
 RETURNING t.user_id,t.id`, id, owner).Scan(&p.UserID, &p.TenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, false, nil
	}
	if err != nil {
		return p, false, err
	}
	p.TenantActive = true
	err = tx.Commit(ctx)
	return p, err == nil, err
}

func (s *Store) RenewReportJob(ctx context.Context, p domain.Principal, id, owner uuid.UUID) error {
	return s.WithPrincipalTx(ctx, p, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE report_generation_jobs SET lease_until=now()+interval '90 seconds' WHERE tenant_id=$1 AND id=$2 AND status='running' AND lease_owner=$3 AND lease_until>now() AND EXISTS(SELECT 1 FROM tenants WHERE id=$1 AND deleted_at IS NULL)`, p.TenantID, id, owner)
		if err == nil && tag.RowsAffected() != 1 {
			return ErrReportJobLeaseLost
		}
		return err
	})
}

func (s *Store) FinishReportJob(ctx context.Context, p domain.Principal, id, owner uuid.UUID, content, code string, usage ...domain.AIUsage) error {
	return s.WithPrincipalTx(ctx, p, func(tx pgx.Tx) error {
		status := "success"
		if code != "" {
			status = "failed"
			content = ""
		}
		tag, err := tx.Exec(ctx, `UPDATE report_generation_jobs SET status=$4::varchar,stage=$4::varchar,content=$5,failure_code=NULLIF($6,''),lease_owner=NULL,lease_until=NULL,updated_at=now() WHERE tenant_id=$1 AND id=$2 AND status='running' AND lease_owner=$3 AND lease_until>now() AND EXISTS(SELECT 1 FROM tenants WHERE id=$1 AND deleted_at IS NULL)`, p.TenantID, id, owner, status, content, code)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrReportJobLeaseLost
		}
		if len(usage) > 0 {
			u := usage[0]
			if _, err = tx.Exec(ctx, `INSERT INTO ai_usage_records(tenant_id,user_id,request_type,input_tokens,output_tokens,model,duration_ms,status,error_code) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, p.TenantID, p.UserID, u.RequestType, u.InputTokens, u.OutputTokens, u.Model, u.Duration.Milliseconds(), u.Status, u.ErrorCode); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `UPDATE scheduled_report_runs r SET status=$3::varchar,error_code=NULLIF($4,''),error_message=CASE WHEN $3::varchar='failed' THEN '后台报告生成失败，请查看任务并重试' ELSE NULL END,finished_at=now() FROM report_generation_jobs j WHERE j.tenant_id=$1 AND j.id=$2 AND r.tenant_id=$1 AND r.id=j.scheduled_run_id`, p.TenantID, id, status, code)
		return err
	})
}

func (s *Store) CancelReportJob(ctx context.Context, p domain.Principal, id uuid.UUID) error {
	return s.WithPrincipalTx(ctx, p, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT status FROM report_generation_jobs WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, p.TenantID, id).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("REPORT_JOB_NOT_FOUND", "报告任务不存在", 404)
		}
		if err != nil {
			return err
		}
		if status != "queued" && status != "running" {
			return apierror.New("REPORT_JOB_FINISHED", "任务已结束", 409)
		}
		if _, err = tx.Exec(ctx, `UPDATE report_generation_jobs SET status='cancelled',stage='cancelled',lease_owner=NULL,lease_until=NULL,updated_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, id); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE scheduled_report_runs r SET status='failed',error_code='REPORT_CANCELLED',error_message='用户取消',finished_at=now() FROM report_generation_jobs j WHERE j.tenant_id=$1 AND j.id=$2 AND r.tenant_id=$1 AND r.id=j.scheduled_run_id`, p.TenantID, id)
		return err
	})
}

// Recovery never silently restarts a generation that might already have emitted
// tokens. Queued jobs are periodically re-signalled through the durable outbox.
func (s *Store) ReconcileReportJobs(ctx context.Context) error {
	tx, err := s.AdminPool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `WITH expired AS (
 UPDATE report_generation_jobs SET status='failed',stage='failed',failure_code='REPORT_WORKER_INTERRUPTED',lease_owner=NULL,lease_until=NULL,updated_at=now()
 WHERE status='running' AND lease_until<now() RETURNING scheduled_run_id
 ) UPDATE scheduled_report_runs SET status='failed',error_code='REPORT_WORKER_INTERRUPTED',error_message='执行中断，请手动重试',finished_at=now() WHERE id IN (SELECT scheduled_run_id FROM expired)`); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT j.id FROM report_generation_jobs j WHERE j.status='queued' AND j.dispatch_at<=now()
 AND NOT EXISTS(SELECT 1 FROM outbox_events e WHERE e.aggregate_type='report' AND e.aggregate_id=j.id::text AND e.processed_at IS NULL)
 ORDER BY j.dispatch_at FOR UPDATE SKIP LOCKED LIMIT 100`)
	if err != nil {
		return err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = reportJobEvent(ctx, tx, id); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE report_generation_jobs SET dispatch_at=now()+interval '1 minute' WHERE id=$1`, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Confirmation locks the job, checks the exact source/target versions, and
// writes the note, revision, citations and confirmation receipt atomically.
func (s *Store) ConfirmReportJob(ctx context.Context, p domain.Principal, id uuid.UUID, title, content string, overwrite bool) (map[string]any, error) {
	var result map[string]any
	err := s.WithPrincipalTx(ctx, p, func(tx pgx.Tx) error {
		j, err := scanReportJob(tx.QueryRow(ctx, `SELECT `+reportJobColumns+` FROM report_generation_jobs WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, p.TenantID, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("REPORT_JOB_NOT_FOUND", "报告任务不存在", 404)
		}
		if err != nil {
			return err
		}
		if j.ConfirmedNoteID != nil {
			result = map[string]any{"id": *j.ConfirmedNoteID}
			return nil
		}
		if j.Status != "success" {
			return apierror.New("REPORT_JOB_NOT_READY", "任务没有可确认的完整草稿", 409)
		}
		// Prevent concurrent source edits between validation and confirmation.
		ids := make([]int32, 0, len(j.Sources))
		for _, source := range j.Sources {
			ids = append(ids, source.ID)
		}
		rows, err := tx.Query(ctx, `SELECT id FROM notes WHERE tenant_id=$1 AND id=ANY($2) ORDER BY id FOR SHARE`, p.TenantID, ids)
		if err != nil {
			return err
		}
		for rows.Next() {
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		var valid bool
		err = tx.QueryRow(ctx, `SELECT source_versions=(SELECT COALESCE(jsonb_object_agg(id::text,updated_at::text),'{}') FROM notes WHERE tenant_id=$1 AND id=ANY($3) AND deleted_at IS NULL) FROM report_generation_jobs WHERE tenant_id=$1 AND id=$2`, p.TenantID, id, ids).Scan(&valid)
		if err != nil {
			return err
		}
		if !valid {
			return apierror.New("REPORT_SOURCES_CHANGED", "来源已修改或删除，请重新生成", 409)
		}
		var targetVersion *time.Time
		err = tx.QueryRow(ctx, `SELECT updated_at FROM notes WHERE tenant_id=$1 AND type=$2 AND note_date=$3::date AND deleted_at IS NULL FOR UPDATE`, p.TenantID, j.Type, j.AnchorDate).Scan(&targetVersion)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		err = tx.QueryRow(ctx, `SELECT target_updated_at IS NOT DISTINCT FROM $3::timestamptz FROM report_generation_jobs WHERE tenant_id=$1 AND id=$2`, p.TenantID, id, targetVersion).Scan(&valid)
		if err != nil {
			return err
		}
		if !valid {
			return apierror.New("REPORT_VERSION_CONFLICT", "报告在生成后已发生变化，请重新生成并核对", 409)
		}
		anchor, _ := time.Parse(time.DateOnly, j.AnchorDate)
		result, err = confirmReportTx(ctx, tx, p, j.Type, anchor, title, content, ids, overwrite, nil, 0)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE report_generation_jobs SET confirmed_note_id=$3,updated_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, id, result["id"])
		return err
	})
	return result, err
}

// Dispatch a claimed schedule and its run/Outbox in the same tenant transaction.
func (s *Store) QueueScheduledReportJob(ctx context.Context, p domain.Principal, task ScheduledTask, owner uuid.UUID, trigger string, anchor, next time.Time) (ReportJob, error) {
	var j ReportJob
	err := s.WithPrincipalTx(ctx, p, func(tx pgx.Tx) error {
		var valid bool
		if err := tx.QueryRow(ctx, `SELECT lease_owner=$3 AND lease_until>now() FROM scheduled_report_tasks WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, p.TenantID, task.ID, owner).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return ErrScheduledLeaseLost
		}
		var busy bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM report_generation_jobs WHERE tenant_id=$1 AND scheduled_task_id=$2 AND status IN ('queued','running'))`, p.TenantID, task.ID).Scan(&busy); err != nil {
			return err
		}
		if busy {
			return apierror.New("SCHEDULED_REPORT_BUSY", "该定时报告已在队列中", 409)
		}
		var err error
		j, err = createReportJobTx(ctx, tx, p, uuid.New(), task.ReportType, anchor)
		if err != nil {
			return err
		}
		var runID int64
		if err = tx.QueryRow(ctx, `INSERT INTO scheduled_report_runs(tenant_id,task_id,status,trigger,lease_owner,started_at) VALUES($1,$2,'running',$3,$4,now()) RETURNING id`, p.TenantID, task.ID, trigger, owner).Scan(&runID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE report_generation_jobs SET scheduled_task_id=$3,scheduled_run_id=$4 WHERE tenant_id=$1 AND id=$2`, p.TenantID, j.ID, task.ID, runID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE scheduled_report_tasks SET next_run_at=$3,last_run_at=now(),lease_owner=NULL,lease_until=NULL,updated_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, task.ID, next)
		return err
	})
	return j, err
}
