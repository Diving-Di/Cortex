package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type RecipeIndexJob struct {
	ID                 int64
	DocumentID         int64
	TargetIndexVersion int
}

func (s *Store) ClaimRecipeIndexJobs(ctx context.Context, owner string, limit int, lease time.Duration) ([]RecipeIndexJob, error) {
	tx, err := s.AdminPool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `WITH candidates AS (
            SELECT j.id
            FROM recipe_index_jobs j
            WHERE (j.status='queued' AND j.next_attempt_at <= now())
               OR (j.status='running' AND j.lease_until < now())
            ORDER BY j.next_attempt_at,j.id
            FOR UPDATE SKIP LOCKED
            LIMIT $1
        )
        UPDATE recipe_index_jobs j SET
            status='running',lease_owner=$2,lease_until=now()+$3::interval,
            attempts=j.attempts+1,updated_at=now()
        FROM candidates c
        WHERE j.id=c.id
        RETURNING j.id,j.document_id,j.target_index_version`,
		limit, owner, lease.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []RecipeIndexJob
	for rows.Next() {
		var item RecipeIndexJob
		if err := rows.Scan(&item.ID, &item.DocumentID, &item.TargetIndexVersion); err != nil {
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

func (s *Store) InsertRecipeIndexJob(ctx context.Context, documentID int64, targetIndexVersion int) error {
	_, err := s.AdminPool.Exec(ctx, `INSERT INTO recipe_index_jobs (document_id,target_index_version)
		VALUES ($1,$2)
		ON CONFLICT (document_id,target_index_version) DO UPDATE SET
			status='queued',next_attempt_at=now(),last_error_code=NULL,updated_at=now()`,
		documentID, targetIndexVersion)
	return err
}

func (s *Store) CompleteRecipeIndex(ctx context.Context, job RecipeIndexJob) error {
	_, err := s.AdminPool.Exec(ctx, `UPDATE recipe_index_jobs SET status='success',lease_owner=NULL,lease_until=NULL,last_error_code=NULL,updated_at=now() WHERE id=$1`, job.ID)
	return err
}

func (s *Store) FailRecipeIndex(ctx context.Context, job RecipeIndexJob, code string, retry bool) error {
	_, err := s.AdminPool.Exec(ctx, `UPDATE recipe_index_jobs SET
		status=CASE WHEN $2 AND attempts < 3 THEN 'queued' ELSE 'failed' END,
		lease_owner=NULL,lease_until=NULL,last_error_code=$3,
		next_attempt_at=now()+make_interval(secs => LEAST(3600, attempts*attempts*10)),
		updated_at=now()
		WHERE id=$1`, job.ID, retry, code)
	return err
}
