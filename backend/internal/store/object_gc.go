package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ObjectGCJob struct {
	ID           int64
	TenantID     uuid.UUID
	Backend, Key string
	Attempt      int
}

func (s *Store) ClaimObjectGC(ctx context.Context) (*ObjectGCJob, error) {
	tx, err := s.AdminPool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var j ObjectGCJob
	err = tx.QueryRow(ctx, `SELECT id,tenant_id,storage_backend,object_key,attempt_count FROM object_gc_jobs WHERE status='queued' AND available_at<=now() ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&j.ID, &j.TenantID, &j.Backend, &j.Key, &j.Attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `UPDATE object_gc_jobs SET status='running',attempt_count=attempt_count+1,updated_at=now() WHERE id=$1`, j.ID); err != nil {
		return nil, err
	}
	return &j, tx.Commit(ctx)
}
func (s *Store) FinishObjectGC(ctx context.Context, id int64, success bool) error {
	if success {
		_, err := s.AdminPool.Exec(ctx, `UPDATE object_gc_jobs SET status='success',last_error_code=NULL,updated_at=now() WHERE id=$1`, id)
		return err
	}
	_, err := s.AdminPool.Exec(ctx, `UPDATE object_gc_jobs SET status='queued',available_at=now()+interval '30 seconds',last_error_code='OBJECT_DELETE_FAILED',updated_at=now() WHERE id=$1`, id)
	return err
}
