package store

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests run in CI/acceptance when MIGRATION_DATABASE_URL points at an
// isolated migrated PostgreSQL database. Unit-only runs skip safely.
func TestOutboxConcurrentClaimAndFencing(t *testing.T) {
	url := os.Getenv("MIGRATION_DATABASE_URL")
	if url == "" {
		t.Skip("MIGRATION_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := &Store{AdminPool: pool, Pool: pool}
	id := uuid.New()
	if _, err = pool.Exec(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload) VALUES($1,'template',$2,'template.test','{}')`, id, uuid.New().String()); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM outbox_events WHERE id=$1`, id)
	var wg sync.WaitGroup
	wg.Add(2)
	claimed := make(chan *OutboxEvent, 2)
	errs := make(chan error, 2)
	for _, owner := range []string{"worker-a", "worker-b"} {
		go func(owner string) {
			defer wg.Done()
			event, e := s.ClaimOutboxEvent(ctx, "template", owner, 500*time.Millisecond)
			claimed <- event
			errs <- e
		}(owner)
	}
	wg.Wait()
	close(claimed)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	count := 0
	owner := ""
	for event := range claimed {
		if event != nil && event.ID == id.String() {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("event claimed %d times", count)
	}
	if err := pool.QueryRow(ctx, `SELECT lease_owner FROM outbox_events WHERE id=$1`, id).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishOutboxEvent(ctx, id.String(), "stale-owner", nil); err == nil {
		t.Fatal("stale owner bypassed completion fence")
	}
	if ok, err := s.RenewOutboxEventLease(ctx, id.String(), owner, time.Second); err != nil || !ok {
		t.Fatalf("renew ok=%v err=%v", ok, err)
	}
}

func TestOutboxExpiredLeaseCanBeReclaimed(t *testing.T) {
	url := os.Getenv("MIGRATION_DATABASE_URL")
	if url == "" {
		t.Skip("MIGRATION_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := &Store{AdminPool: pool, Pool: pool}
	id := uuid.New()
	if _, err = pool.Exec(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload,lease_owner,lease_until) VALUES($1,'template',$2,'template.test','{}','crashed',now()-interval '1 second')`, id, uuid.New().String()); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM outbox_events WHERE id=$1`, id)
	event, err := s.ClaimOutboxEvent(ctx, "template", "replacement", time.Second)
	if err != nil || event == nil || event.ID != id.String() {
		t.Fatalf("reclaim event=%v err=%v", event, err)
	}
}
