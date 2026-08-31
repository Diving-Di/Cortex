package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ObjectGCJob struct {
	ID            int64
	TenantID      uuid.UUID
	LeaseOwner    uuid.UUID
	Backend, Key  string
	ObjectVersion string
	Attempt       int
}

func (s *Store) ClaimObjectGC(ctx context.Context, leaseDuration time.Duration) (*ObjectGCJob, error) {
	tx, err := s.AdminPool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	j := ObjectGCJob{LeaseOwner: uuid.New()}
	err = tx.QueryRow(ctx, `SELECT id,tenant_id,storage_backend,object_key,coalesce(object_version,''),attempt_count
		FROM object_gc_jobs
		WHERE (status='queued' AND available_at<=now()) OR (status='running' AND lease_expires_at<=now())
		ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&j.ID, &j.TenantID, &j.Backend, &j.Key, &j.ObjectVersion, &j.Attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `UPDATE object_gc_jobs SET status='running',attempt_count=attempt_count+1,lease_owner=$2,lease_expires_at=now()+make_interval(secs => $3),updated_at=now() WHERE id=$1`, j.ID, j.LeaseOwner, leaseDuration.Seconds()); err != nil {
		return nil, err
	}
	return &j, tx.Commit(ctx)
}
func (s *Store) FinishObjectGC(ctx context.Context, job ObjectGCJob, success bool) error {
	tx, err := s.AdminPool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if !success {
		_, err = tx.Exec(ctx, `UPDATE object_gc_jobs SET status='queued',available_at=now()+interval '30 seconds',lease_owner=NULL,lease_expires_at=NULL,last_error_code='OBJECT_DELETE_FAILED',updated_at=now() WHERE id=$1 AND status='running' AND lease_owner=$2`, job.ID, job.LeaseOwner)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	tag, err := tx.Exec(ctx, `UPDATE object_gc_jobs SET status='success',lease_owner=NULL,lease_expires_at=NULL,last_error_code=NULL,updated_at=now() WHERE id=$1 AND status='running' AND lease_owner=$2`, job.ID, job.LeaseOwner)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}
	_, err = tx.Exec(ctx, `DELETE FROM attachments WHERE tenant_id=$1 AND deleted_at IS NOT NULL AND storage_backend=$2 AND coalesce(object_key,stored_path)=$3 AND nullif(object_version,'') IS NOT DISTINCT FROM nullif($4,'')`, job.TenantID, job.Backend, job.Key, job.ObjectVersion)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
