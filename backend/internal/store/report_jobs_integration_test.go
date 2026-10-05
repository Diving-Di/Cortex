package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func reportJobFixture(t *testing.T) (*Store, domain.Principal, int32, time.Time) {
	t.Helper()
	appURL, adminURL := os.Getenv("DATABASE_URL"), os.Getenv("MIGRATION_DATABASE_URL")
	if appURL == "" || adminURL == "" {
		t.Skip("database URLs are not configured")
	}
	ctx := context.Background()
	app, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		app.Close()
		t.Fatal(err)
	}
	s := &Store{Pool: app, AdminPool: admin}
	p := domain.Principal{TenantID: uuid.New(), TenantActive: true}
	name := "report_" + uuid.NewString()
	if err = admin.QueryRow(ctx, `INSERT INTO users(username,email,password_hash) VALUES($1,$2,'test') RETURNING id`, name, name+"@example.invalid").Scan(&p.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(ctx, `INSERT INTO tenants(id,user_id,name) VALUES($1,$2,'report test')`, p.TenantID, p.UserID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		admin.Exec(ctx, `DELETE FROM outbox_events WHERE aggregate_type='report' AND aggregate_id IN (SELECT id::text FROM report_generation_jobs WHERE tenant_id=$1)`, p.TenantID)
		admin.Exec(ctx, `DELETE FROM tenants WHERE id=$1`, p.TenantID)
		admin.Exec(ctx, `DELETE FROM users WHERE id=$1`, p.UserID)
		app.Close()
		admin.Close()
	})
	anchor := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	var noteID int32
	if err = admin.QueryRow(ctx, `INSERT INTO notes(tenant_id,created_by,updated_by,type,title,content,note_date) VALUES($1,$2,$2,'daily','source','事实',$3) RETURNING id`, p.TenantID, p.UserID, anchor).Scan(&noteID); err != nil {
		t.Fatal(err)
	}
	return s, p, noteID, anchor
}

func assertReportCode(t *testing.T, err error, code string) {
	t.Helper()
	var appErr *apierror.Error
	if !errors.As(err, &appErr) || appErr.Code != code {
		t.Fatalf("err=%v want %s", err, code)
	}
}

func TestReportJobIdempotencyRLSClaimAndConfirmation(t *testing.T) {
	s, p, noteID, anchor := reportJobFixture(t)
	ctx := context.Background()
	requestID := uuid.New()
	j, err := s.CreateReportJob(ctx, p, requestID, "monthly", anchor)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := s.CreateReportJob(ctx, p, requestID, "monthly", anchor)
	if err != nil || duplicate.ID != j.ID {
		t.Fatalf("idempotency %v", err)
	}
	_, err = s.CreateReportJob(ctx, p, requestID, "weekly", anchor)
	assertReportCode(t, err, "IDEMPOTENCY_CONFLICT")
	var events int
	if err = s.AdminPool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_type='report' AND aggregate_id=$1`, j.ID.String()).Scan(&events); err != nil || events != 1 {
		t.Fatalf("events=%d err=%v", events, err)
	}
	stranger := p
	stranger.TenantID = uuid.New()
	_, err = s.GetReportJob(ctx, stranger, j.ID)
	assertReportCode(t, err, "REPORT_JOB_NOT_FOUND")
	if _, err = s.ConfirmReportJob(ctx, stranger, j.ID, "title", "body", false); err == nil {
		t.Fatal("cross tenant confirmation")
	}
	var wg sync.WaitGroup
	owners := make(chan uuid.UUID, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			owner := uuid.New()
			_, ok, e := s.ClaimReportJob(ctx, j.ID, owner)
			errs <- e
			if ok {
				owners <- owner
			}
		}()
	}
	wg.Wait()
	close(owners)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var owner uuid.UUID
	claims := 0
	for o := range owners {
		owner = o
		claims++
	}
	if claims != 1 {
		t.Fatalf("claims=%d", claims)
	}
	if err = s.FinishReportJob(ctx, p, j.ID, uuid.New(), "bad", ""); !errors.Is(err, ErrReportJobLeaseLost) {
		t.Fatal(err)
	}
	content := fmt.Sprintf("事实 [#%d]", noteID)
	if err = s.FinishReportJob(ctx, p, j.ID, owner, content, ""); err != nil {
		t.Fatal(err)
	}
	result, err := s.ConfirmReportJob(ctx, p, j.ID, "月报", content, false)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.ConfirmReportJob(ctx, p, j.ID, "月报", content, false)
	if err != nil || again["id"] != result["id"] {
		t.Fatalf("confirm replay %v", err)
	}
	var sourceCount int
	if err = s.AdminPool.QueryRow(ctx, `SELECT count(*) FROM report_sources WHERE tenant_id=$1 AND report_note_id=$2`, p.TenantID, result["id"]).Scan(&sourceCount); err != nil || sourceCount != 1 {
		t.Fatalf("sources=%d err=%v", sourceCount, err)
	}
}

func TestReportJobRecoveryAndSourceConflict(t *testing.T) {
	s, p, sourceID, anchor := reportJobFixture(t)
	ctx := context.Background()
	j, err := s.CreateReportJob(ctx, p, uuid.New(), "monthly", anchor)
	if err != nil {
		t.Fatal(err)
	}
	owner := uuid.New()
	_, ok, err := s.ClaimReportJob(ctx, j.ID, owner)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if _, err = s.AdminPool.Exec(ctx, `UPDATE report_generation_jobs SET lease_until=now()-interval '1 second' WHERE id=$1`, j.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.ReconcileReportJobs(ctx); err != nil {
		t.Fatal(err)
	}
	failed, err := s.GetReportJob(ctx, p, j.ID)
	if err != nil || failed.Status != "failed" || failed.FailureCode == nil || *failed.FailureCode != "REPORT_WORKER_INTERRUPTED" {
		t.Fatalf("%+v err=%v", failed, err)
	}
	if err = s.FinishReportJob(ctx, p, j.ID, owner, "late", ""); !errors.Is(err, ErrReportJobLeaseLost) {
		t.Fatal(err)
	}
	j, err = s.CreateReportJob(ctx, p, uuid.New(), "monthly", anchor)
	if err != nil {
		t.Fatal(err)
	}
	_, ok, err = s.ClaimReportJob(ctx, j.ID, owner)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if err = s.FinishReportJob(ctx, p, j.ID, owner, "事实", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdminPool.Exec(ctx, `UPDATE notes SET content='修改后',updated_at=now() WHERE id=$1`, sourceID); err != nil {
		t.Fatal(err)
	}
	err = s.ValidateReportJobSources(ctx, p, j.ID)
	assertReportCode(t, err, "REPORT_SOURCES_CHANGED")
	_, err = s.ConfirmReportJob(ctx, p, j.ID, "月报", "事实", false)
	assertReportCode(t, err, "REPORT_SOURCES_CHANGED")
}

func TestReportJobQueueRecoveryCancelAndAdmission(t *testing.T) {
	s, p, _, anchor := reportJobFixture(t)
	ctx := context.Background()
	var ids []uuid.UUID
	for i := 0; i < 3; i++ {
		j, err := s.CreateReportJob(ctx, p, uuid.New(), "monthly", anchor)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, j.ID)
	}
	_, err := s.CreateReportJob(ctx, p, uuid.New(), "monthly", anchor)
	assertReportCode(t, err, "REPORT_QUEUE_FULL")
	if _, err = s.AdminPool.Exec(ctx, `UPDATE outbox_events SET processed_at=now() WHERE aggregate_id=$1`, ids[0].String()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdminPool.Exec(ctx, `UPDATE report_generation_jobs SET dispatch_at=now()-interval '1 minute' WHERE id=$1`, ids[0]); err != nil {
		t.Fatal(err)
	}
	if err = s.ReconcileReportJobs(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.AdminPool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id=$1 AND processed_at IS NULL`, ids[0].String()).Scan(&count); err != nil || count != 1 {
		t.Fatalf("recovery count=%d err=%v", count, err)
	}
	if err = s.CancelReportJob(ctx, p, ids[0]); err != nil {
		t.Fatal(err)
	}
	_, ok, err := s.ClaimReportJob(ctx, ids[0], uuid.New())
	if err != nil || ok {
		t.Fatalf("cancelled claimed=%t err=%v", ok, err)
	}
	if _, err = s.CreateReportJob(ctx, p, uuid.New(), "monthly", anchor); err != nil {
		t.Fatal(err)
	}
}

func TestScheduledReportDispatchIsDurableAndProducesOneRun(t *testing.T) {
	s, p, _, anchor := reportJobFixture(t)
	ctx := context.Background()
	task, err := s.CreateScheduledTask(ctx, p, "monthly", 20, 0, "Asia/Shanghai", anchor)
	if err != nil {
		t.Fatal(err)
	}
	owner := uuid.New()
	if err = s.AcquireScheduledTaskLease(ctx, p, task.ID, owner, time.Minute); err != nil {
		t.Fatal(err)
	}
	job, err := s.QueueScheduledReportJob(ctx, p, task, owner, "manual", anchor, anchor.AddDate(0, 1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AcquireScheduledTaskLease(ctx, p, task.ID, uuid.New(), time.Minute); !errors.Is(err, ErrScheduledLeaseLost) {
		t.Fatalf("duplicate retry acquired lease: %v", err)
	}
	runs, err := s.ListScheduledRuns(ctx, p, task.ID)
	if err != nil || len(runs) != 1 || runs[0].Status != "running" {
		t.Fatalf("runs=%v err=%v", runs, err)
	}
	if _, err = s.QueueScheduledReportJob(ctx, p, task, owner, "manual", anchor, anchor); err == nil {
		t.Fatal("stale schedule dispatched")
	}
	// A new process can consume the persisted job using only its ID.
	worker := &Store{Pool: s.Pool, AdminPool: s.AdminPool}
	workerOwner := uuid.New()
	_, ok, err := worker.ClaimReportJob(ctx, job.ID, workerOwner)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if err = worker.FinishReportJob(ctx, p, job.ID, workerOwner, "草稿", ""); err != nil {
		t.Fatal(err)
	}
	runs, err = s.ListScheduledRuns(ctx, p, task.ID)
	if err != nil || len(runs) != 1 || runs[0].Status != "success" || runs[0].ReportNoteID != nil {
		t.Fatalf("runs=%v err=%v", runs, err)
	}
}

func TestReportJobTargetConflictAndCancelledOwner(t *testing.T) {
	s, p, _, anchor := reportJobFixture(t)
	ctx := context.Background()
	job, err := s.CreateReportJob(ctx, p, uuid.New(), "monthly", anchor)
	if err != nil {
		t.Fatal(err)
	}
	owner := uuid.New()
	_, ok, err := s.ClaimReportJob(ctx, job.ID, owner)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if err = s.CancelReportJob(ctx, p, job.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.RenewReportJob(ctx, p, job.ID, owner); !errors.Is(err, ErrReportJobLeaseLost) {
		t.Fatal(err)
	}
	if err = s.FinishReportJob(ctx, p, job.ID, owner, "late", ""); !errors.Is(err, ErrReportJobLeaseLost) {
		t.Fatal(err)
	}
	job, err = s.CreateReportJob(ctx, p, uuid.New(), "monthly", anchor)
	if err != nil {
		t.Fatal(err)
	}
	_, ok, err = s.ClaimReportJob(ctx, job.ID, owner)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if err = s.FinishReportJob(ctx, p, job.ID, owner, "草稿", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdminPool.Exec(ctx, `INSERT INTO notes(tenant_id,created_by,updated_by,type,title,content,note_date) VALUES($1,$2,$2,'monthly','other','other','2026-10-01')`, p.TenantID, p.UserID); err != nil {
		t.Fatal(err)
	}
	_, err = s.ConfirmReportJob(ctx, p, job.ID, "title", "草稿", true)
	assertReportCode(t, err, "REPORT_VERSION_CONFLICT")
}
