package store

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"cortex/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestScheduledTaskLeaseClaimReclaimAndFencing(t *testing.T) {
	appURL, adminURL := os.Getenv("DATABASE_URL"), os.Getenv("MIGRATION_DATABASE_URL")
	if appURL == "" || adminURL == "" {
		t.Skip("database URLs are not configured")
	}
	ctx := context.Background()
	app, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	s := &Store{Pool: app, AdminPool: admin}
	tenantID := uuid.New()
	username := "scheduled_" + uuid.NewString()
	var userID, taskID int32
	if err := admin.QueryRow(ctx, `INSERT INTO users(username,email,password_hash) VALUES($1,$2,'test') RETURNING id`, username, username+"@example.invalid").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)
	if _, err := admin.Exec(ctx, `INSERT INTO tenants(id,user_id,name) VALUES($1,$2,'scheduler integration')`, tenantID, userID); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, `DELETE FROM tenants WHERE id=$1`, tenantID)
	if err := admin.QueryRow(ctx, `INSERT INTO scheduled_report_tasks(tenant_id,created_by,report_type,hour,minute,timezone,next_run_at) VALUES($1,$2,'daily',0,0,'Asia/Shanghai',now()-interval '1 minute') RETURNING id`, tenantID, userID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	principal := domain.Principal{UserID: userID, TenantID: tenantID, TenantActive: true}
	ownerA, ownerB := uuid.New(), uuid.New()
	var wg sync.WaitGroup
	wg.Add(2)
	counts := make(chan int, 2)
	errs := make(chan error, 2)
	for _, owner := range []uuid.UUID{ownerA, ownerB} {
		go func(o uuid.UUID) {
			defer wg.Done()
			items, e := s.ClaimDueScheduledTasks(ctx, o, 1, time.Minute)
			counts <- len(items)
			errs <- e
		}(owner)
	}
	wg.Wait()
	close(counts)
	close(errs)
	total := 0
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	for n := range counts {
		total += n
	}
	if total != 1 {
		t.Fatalf("claimed %d times", total)
	}
	var activeOwner uuid.UUID
	if err := admin.QueryRow(ctx, `SELECT lease_owner FROM scheduled_report_tasks WHERE id=$1`, taskID).Scan(&activeOwner); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartScheduledRun(ctx, principal, taskID, "scheduled", uuid.New()); !errors.Is(err, ErrScheduledLeaseLost) {
		t.Fatalf("stale start err=%v", err)
	}
	oldRunID, err := s.StartScheduledRun(ctx, principal, taskID, "scheduled", activeOwner)
	if err != nil {
		t.Fatal(err)
	}
	replacementOwner := ownerA
	if replacementOwner == activeOwner {
		replacementOwner = ownerB
	}
	if _, err := admin.Exec(ctx, `UPDATE scheduled_report_tasks SET lease_until=now()-interval '1 second' WHERE id=$1`, taskID); err != nil {
		t.Fatal(err)
	}
	items, err := s.ClaimDueScheduledTasks(ctx, replacementOwner, 1, time.Minute)
	if err != nil || len(items) != 1 {
		t.Fatalf("reclaim=%d err=%v", len(items), err)
	}
	var oldStatus, oldCode string
	if err := admin.QueryRow(ctx, `SELECT status,coalesce(error_code,'') FROM scheduled_report_runs WHERE id=$1`, oldRunID).Scan(&oldStatus, &oldCode); err != nil {
		t.Fatal(err)
	}
	if oldStatus != "failed" || oldCode != "SCHEDULED_LEASE_EXPIRED" {
		t.Fatalf("old run=%s/%s", oldStatus, oldCode)
	}
	runID, err := s.StartScheduledRun(ctx, principal, taskID, "scheduled", replacementOwner)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.GetScheduledTask(ctx, principal, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishScheduledRun(ctx, principal, task, runID, nil, nil, time.Now().Add(time.Hour), activeOwner); !errors.Is(err, ErrScheduledLeaseLost) {
		t.Fatalf("stale finish err=%v", err)
	}
	if err := s.FinishScheduledRun(ctx, principal, task, runID, nil, nil, time.Now().Add(time.Hour), replacementOwner); err != nil {
		t.Fatal(err)
	}
	var leaseOwner *uuid.UUID
	if err := admin.QueryRow(ctx, `SELECT lease_owner FROM scheduled_report_tasks WHERE id=$1`, taskID).Scan(&leaseOwner); err != nil {
		t.Fatal(err)
	}
	if leaseOwner != nil {
		t.Fatal("lease not cleared")
	}
}
