package store

import (
    "context"
    "errors"
    "fmt"
    "time"

    "diary-listener/backend/internal/apierror"
    "diary-listener/backend/internal/domain"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
)

type ScheduledTask struct {
    ID         int32   `json:"id"`
    ReportType string  `json:"report_type"`
    Hour       int32   `json:"hour"`
    Minute     int32   `json:"minute"`
    Timezone   string  `json:"timezone"`
    Status     string  `json:"status"`
    NextRunAt  string  `json:"next_run_at"`
    LastRunAt  *string `json:"last_run_at"`
    CreatedAt  string  `json:"created_at"`
}

type ScheduledRun struct {
    ID           int64   `json:"id"`
    Status       string  `json:"status"`
    Trigger      string  `json:"trigger"`
    ReportNoteID *int32  `json:"report_note_id"`
    ErrorCode    *string `json:"error_code"`
    ErrorMessage *string `json:"error_message"`
    StartedAt    string  `json:"started_at"`
    FinishedAt   *string `json:"finished_at"`
}

type ClaimedScheduledTask struct {
    TaskID   int32
    TenantID uuid.UUID
    UserID   int32
}

func (s *Store) ClaimDueScheduledTasks(ctx context.Context, limit int) ([]ClaimedScheduledTask, error) {
    tx, err := s.AdminPool.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        return nil, err
    }
    defer func() { _ = tx.Rollback(ctx) }()
    rows, err := tx.Query(ctx, `SELECT id,tenant_id,created_by FROM scheduled_report_tasks
        WHERE status='enabled' AND next_run_at<=now()
        ORDER BY next_run_at FOR UPDATE SKIP LOCKED LIMIT $1`, limit)
    if err != nil {
        return nil, err
    }
    var claimed []ClaimedScheduledTask
    for rows.Next() {
        var item ClaimedScheduledTask
        if err := rows.Scan(&item.TaskID, &item.TenantID, &item.UserID); err != nil {
            rows.Close()
            return nil, err
        }
        claimed = append(claimed, item)
    }
    if err := rows.Err(); err != nil {
        rows.Close()
        return nil, err
    }
    rows.Close()
    for _, item := range claimed {
        if _, err := tx.Exec(ctx, `UPDATE scheduled_report_tasks
            SET next_run_at=now()+interval '5 minutes',updated_at=now() WHERE id=$1`, item.TaskID); err != nil {
            return nil, err
        }
    }
    if err := tx.Commit(ctx); err != nil {
        return nil, err
    }
    return claimed, nil
}

func (s *Store) ListScheduledTasks(ctx context.Context, principal domain.Principal) ([]ScheduledTask, error) {
    var result []ScheduledTask
    err := s.WithTx(ctx, func(tx pgx.Tx) error {
        if err := setTenant(ctx, tx, principal); err != nil {
            return err
        }
        rows, err := tx.Query(ctx, `SELECT id,report_type,hour,minute,timezone,status,next_run_at,last_run_at,created_at
            FROM scheduled_report_tasks WHERE tenant_id=$1 ORDER BY id`, principal.TenantID)
        if err != nil {
            return err
        }
        defer rows.Close()
        for rows.Next() {
            item, err := scanScheduledTask(rows)
            if err != nil {
                return err
            }
            result = append(result, item)
        }
        return rows.Err()
    })
    return result, err
}

func (s *Store) CreateScheduledTask(
    ctx context.Context,
    principal domain.Principal,
    reportType string,
    hour int,
    minute int,
    timezone string,
    nextRun time.Time,
) (ScheduledTask, error) {
    var result ScheduledTask
    err := s.WithTx(ctx, func(tx pgx.Tx) error {
        if err := setTenant(ctx, tx, principal); err != nil {
            return err
        }
        item, err := scanScheduledTask(tx.QueryRow(ctx, `INSERT INTO scheduled_report_tasks
            (tenant_id,created_by,report_type,hour,minute,timezone,next_run_at)
            VALUES ($1,$2,$3,$4,$5,$6,$7)
            RETURNING id,report_type,hour,minute,timezone,status,next_run_at,last_run_at,created_at`,
            principal.TenantID, principal.UserID, reportType, hour, minute, timezone, nextRun,
        ))
        if err != nil {
            return err
        }
        result = item
        return auditResource(ctx, tx, principal, "scheduled_report.create", "scheduled_report_task", fmt.Sprint(item.ID))
    })
    return result, err
}

func (s *Store) SetScheduledTaskStatus(
    ctx context.Context,
    principal domain.Principal,
    taskID int32,
    enabled bool,
    nextRun time.Time,
) (ScheduledTask, error) {
    var result ScheduledTask
    err := s.WithTx(ctx, func(tx pgx.Tx) error {
        if err := setTenant(ctx, tx, principal); err != nil {
            return err
        }
        status := "disabled"
        if enabled {
            status = "enabled"
        }
        item, err := scanScheduledTask(tx.QueryRow(ctx, `UPDATE scheduled_report_tasks SET
            status=$1,next_run_at=CASE WHEN $1='enabled' THEN $2 ELSE next_run_at END,updated_at=now()
            WHERE tenant_id=$3 AND id=$4
            RETURNING id,report_type,hour,minute,timezone,status,next_run_at,last_run_at,created_at`,
            status, nextRun, principal.TenantID, taskID,
        ))
        if errors.Is(err, pgx.ErrNoRows) {
            return apierror.New("SCHEDULED_REPORT_NOT_FOUND", "定时报告任务不存在", 404)
        }
        if err != nil {
            return err
        }
        result = item
        return auditResource(ctx, tx, principal, "scheduled_report.status", "scheduled_report_task", fmt.Sprint(taskID))
    })
    return result, err
}

func (s *Store) GetScheduledTask(ctx context.Context, principal domain.Principal, taskID int32) (ScheduledTask, error) {
    var result ScheduledTask
    err := s.WithTx(ctx, func(tx pgx.Tx) error {
        if err := setTenant(ctx, tx, principal); err != nil {
            return err
        }
        item, err := scanScheduledTask(tx.QueryRow(ctx, `SELECT id,report_type,hour,minute,timezone,status,next_run_at,last_run_at,created_at
            FROM scheduled_report_tasks WHERE tenant_id=$1 AND id=$2`, principal.TenantID, taskID))
        if errors.Is(err, pgx.ErrNoRows) {
            return apierror.New("SCHEDULED_REPORT_NOT_FOUND", "定时报告任务不存在", 404)
        }
        result = item
        return err
    })
    return result, err
}

func (s *Store) ListScheduledRuns(ctx context.Context, principal domain.Principal, taskID int32) ([]ScheduledRun, error) {
    if _, err := s.GetScheduledTask(ctx, principal, taskID); err != nil {
        return nil, err
    }
    var result []ScheduledRun
    err := s.WithTx(ctx, func(tx pgx.Tx) error {
        if err := setTenant(ctx, tx, principal); err != nil {
            return err
        }
        rows, err := tx.Query(ctx, `SELECT id,status,trigger,report_note_id,error_code,error_message,started_at,finished_at
            FROM scheduled_report_runs WHERE tenant_id=$1 AND task_id=$2 ORDER BY started_at DESC LIMIT 50`,
            principal.TenantID, taskID,
        )
        if err != nil {
            return err
        }
        defer rows.Close()
        for rows.Next() {
            var item ScheduledRun
            var started time.Time
            var finished *time.Time
            if err := rows.Scan(
                &item.ID, &item.Status, &item.Trigger, &item.ReportNoteID,
                &item.ErrorCode, &item.ErrorMessage, &started, &finished,
            ); err != nil {
                return err
            }
            item.StartedAt = started.Format(time.RFC3339Nano)
            if finished != nil {
                value := finished.Format(time.RFC3339Nano)
                item.FinishedAt = &value
            }
            result = append(result, item)
        }
        return rows.Err()
    })
    return result, err
}

func (s *Store) StartScheduledRun(ctx context.Context, principal domain.Principal, taskID int32, trigger string) (int64, error) {
    var runID int64
    err := s.WithTx(ctx, func(tx pgx.Tx) error {
        if err := setTenant(ctx, tx, principal); err != nil {
            return err
        }
        return tx.QueryRow(ctx, `INSERT INTO scheduled_report_runs
            (tenant_id,task_id,status,trigger,started_at) VALUES ($1,$2,'running',$3,now()) RETURNING id`,
            principal.TenantID, taskID, trigger,
        ).Scan(&runID)
    })
    return runID, err
}

func (s *Store) FinishScheduledRun(
    ctx context.Context,
    principal domain.Principal,
    task ScheduledTask,
    runID int64,
    reportNoteID *int32,
    runErr error,
    nextRun time.Time,
) error {
    return s.WithTx(ctx, func(tx pgx.Tx) error {
        if err := setTenant(ctx, tx, principal); err != nil {
            return err
        }
        status := "success"
        var code, message *string
        if runErr != nil {
            status = "failed"
            value := "SCHEDULED_REPORT_FAILED"
            var appErr *apierror.Error
            if errors.As(runErr, &appErr) {
                value = appErr.Code
            }
            text := truncateText(runErr.Error(), 1000)
            code, message = &value, &text
        }
        if _, err := tx.Exec(ctx, `UPDATE scheduled_report_runs SET
            status=$1,report_note_id=$2,error_code=$3,error_message=$4,finished_at=now()
            WHERE tenant_id=$5 AND id=$6`,
            status, reportNoteID, code, message, principal.TenantID, runID,
        ); err != nil {
            return err
        }
        _, err := tx.Exec(ctx, `UPDATE scheduled_report_tasks SET
            last_run_at=now(),next_run_at=$1,updated_at=now() WHERE tenant_id=$2 AND id=$3`,
            nextRun, principal.TenantID, task.ID,
        )
        return err
    })
}

type scheduledScanner interface {
    Scan(...any) error
}

func scanScheduledTask(row scheduledScanner) (ScheduledTask, error) {
    var item ScheduledTask
    var next, created time.Time
    var last *time.Time
    err := row.Scan(
        &item.ID, &item.ReportType, &item.Hour, &item.Minute, &item.Timezone,
        &item.Status, &next, &last, &created,
    )
    item.NextRunAt = next.Format(time.RFC3339Nano)
    item.CreatedAt = created.Format(time.RFC3339Nano)
    if last != nil {
        value := last.Format(time.RFC3339Nano)
        item.LastRunAt = &value
    }
    return item, err
}
