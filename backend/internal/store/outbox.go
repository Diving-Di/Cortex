package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type OutboxEvent struct {
	ID, AggregateType, AggregateID, EventType string
	OccurredAt                                time.Time
	Payload                                   json.RawMessage
}

type MarketplaceMetrics struct {
	PendingOutbox                             int64
	OutboxLagSeconds                          float64
	QueuedClaims, RunningClaims, FailedClaims int64
	PointAccountsDrifted                      int64
	EventSlotDrifted                          int64
	SucceededClaimsInvalid                    int64
}

type OperationsMetrics struct {
	KnowledgeQueued, KnowledgeRunning, KnowledgeFailed int64
	KnowledgeOldestQueuedSeconds                       float64
	ScheduledDue, ScheduledRunning, ScheduledFailed    int64
	ScheduledOldestDueSeconds                          float64
}

type TemplateRankingProjection struct {
	PublicID      string
	PublishedAt   time.Time
	TrendingScore float64
}
type TemplateEventProjection struct {
	PublicID                  string
	Published                 bool
	PublishedAt               time.Time
	TrendingScore, DailyScore float64
}

func (s *Store) GetTemplateEventProjection(ctx context.Context, publicID string) (TemplateEventProjection, error) {
	var x TemplateEventProjection
	x.PublicID = publicID
	err := s.AdminPool.QueryRow(ctx, `SELECT p.status='published',p.published_at,((COALESCE(st.view_count,0)+3*COALESCE(st.like_count,0)+5*COALESCE(st.favorite_count,0)+8*COALESCE(st.usage_count,0))/power(2.0,GREATEST(0,extract(epoch FROM now()-p.published_at))/604800.0))::double precision,(SELECT count(*)::double precision FROM outbox_events oe WHERE oe.aggregate_type='template' AND oe.aggregate_id=p.public_template_id::text AND oe.event_type='template.viewed' AND (oe.occurred_at AT TIME ZONE 'Asia/Shanghai')::date=(now() AT TIME ZONE 'Asia/Shanghai')::date) FROM published_template_snapshots p LEFT JOIN template_public_stats st ON st.public_template_id=p.public_template_id WHERE p.public_template_id=$1::uuid ORDER BY p.version DESC LIMIT 1`, publicID).Scan(&x.Published, &x.PublishedAt, &x.TrendingScore, &x.DailyScore)
	if errors.Is(err, pgx.ErrNoRows) {
		return x, nil
	}
	return x, err
}

func (s *Store) ListTemplateRankingProjections(ctx context.Context) ([]TemplateRankingProjection, error) {
	rows, err := s.AdminPool.Query(ctx, `SELECT p.public_template_id::text,p.published_at,((COALESCE(st.view_count,0)+3*COALESCE(st.like_count,0)+5*COALESCE(st.favorite_count,0)+8*COALESCE(st.usage_count,0))/power(2.0,GREATEST(0,extract(epoch FROM now()-p.published_at))/604800.0))::double precision FROM published_template_snapshots p LEFT JOIN template_public_stats st ON st.public_template_id=p.public_template_id WHERE p.status='published'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []TemplateRankingProjection
	for rows.Next() {
		var x TemplateRankingProjection
		if err := rows.Scan(&x.PublicID, &x.PublishedAt, &x.TrendingScore); err != nil {
			return nil, err
		}
		result = append(result, x)
	}
	return result, rows.Err()
}

func (s *Store) MarkTemplateOutboxRebuilt(ctx context.Context, rebuiltAt time.Time) error {
	_, err := s.AdminPool.Exec(ctx, `UPDATE outbox_events SET processed_at=now(),lease_owner=NULL,lease_until=NULL,last_error_code=NULL WHERE aggregate_type='template' AND processed_at IS NULL AND occurred_at<=$1`, rebuiltAt)
	return err
}

func (s *Store) GetMarketplaceMetrics(ctx context.Context) (MarketplaceMetrics, error) {
	var m MarketplaceMetrics
	err := s.AdminPool.QueryRow(ctx, `SELECT
		count(*) FILTER(WHERE processed_at IS NULL),
		COALESCE(extract(epoch FROM now()-min(occurred_at) FILTER(WHERE processed_at IS NULL)),0),
		(SELECT count(*) FROM ai_flash_claims WHERE status='queued'),
		(SELECT count(*) FROM ai_flash_claims WHERE status='running'),
		(SELECT count(*) FROM ai_flash_claims WHERE status='failed'),
		(SELECT count(*) FROM ai_point_accounts a WHERE a.held_points<>(SELECT COALESCE(sum(CASE entry_type WHEN 'hold' THEN points WHEN 'capture' THEN -points WHEN 'release' THEN -points ELSE 0 END),0) FROM ai_point_ledger l WHERE l.tenant_id=a.tenant_id AND l.period_start=a.period_start) OR a.consumed_points<>(SELECT COALESCE(sum(points) FILTER(WHERE entry_type='capture'),0) FROM ai_point_ledger l WHERE l.tenant_id=a.tenant_id AND l.period_start=a.period_start)),
		(SELECT count(*) FROM ai_flash_events e WHERE e.claimed_slots<>(SELECT count(*) FROM ai_flash_claims c WHERE c.event_id=e.id)),
		(SELECT count(*) FROM ai_flash_claims c LEFT JOIN notes n ON n.id=c.report_note_id AND n.tenant_id=c.tenant_id WHERE c.status='succeeded' AND c.report_note_id IS NOT NULL AND (n.id IS NULL OR NOT EXISTS(SELECT 1 FROM report_sources r WHERE r.tenant_id=c.tenant_id AND r.report_note_id=c.report_note_id))) FROM outbox_events`).Scan(&m.PendingOutbox, &m.OutboxLagSeconds, &m.QueuedClaims, &m.RunningClaims, &m.FailedClaims, &m.PointAccountsDrifted, &m.EventSlotDrifted, &m.SucceededClaimsInvalid)
	return m, err
}

func (s *Store) GetOperationsMetrics(ctx context.Context) (OperationsMetrics, error) {
	var m OperationsMetrics
	err := s.AdminPool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM knowledge_index_jobs WHERE status='queued'),
		(SELECT count(*) FROM knowledge_index_jobs WHERE status='running'),
		(SELECT count(*) FROM knowledge_index_jobs WHERE status='failed'),
		(SELECT COALESCE(extract(epoch FROM now()-min(available_at)),0) FROM knowledge_index_jobs WHERE status='queued' AND available_at<=now()),
		(SELECT count(*) FROM scheduled_report_tasks WHERE status='enabled' AND next_run_at<=now()),
		(SELECT count(*) FROM scheduled_report_runs WHERE status='running'),
		(SELECT count(*) FROM scheduled_report_runs WHERE status='failed'),
		(SELECT COALESCE(extract(epoch FROM now()-min(next_run_at)),0) FROM scheduled_report_tasks WHERE status='enabled' AND next_run_at<=now())`).Scan(
		&m.KnowledgeQueued, &m.KnowledgeRunning, &m.KnowledgeFailed, &m.KnowledgeOldestQueuedSeconds,
		&m.ScheduledDue, &m.ScheduledRunning, &m.ScheduledFailed, &m.ScheduledOldestDueSeconds,
	)
	return m, err
}

func (s *Store) ClaimOutboxEvent(ctx context.Context, aggregateType, owner string, lease time.Duration) (*OutboxEvent, error) {
	if aggregateType == "" || owner == "" || lease <= 0 {
		return nil, errors.New("invalid outbox consumer")
	}
	tx, err := s.AdminPool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id uuid.UUID
	var event OutboxEvent
	err = tx.QueryRow(ctx, `SELECT id,aggregate_type,aggregate_id,event_type,occurred_at,payload FROM outbox_events
		WHERE aggregate_type=$1 AND processed_at IS NULL AND available_at<=now() AND (lease_until IS NULL OR lease_until<now())
		ORDER BY occurred_at FOR UPDATE SKIP LOCKED LIMIT 1`, aggregateType).Scan(&id, &event.AggregateType, &event.AggregateID, &event.EventType, &event.OccurredAt, &event.Payload)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	event.ID = id.String()
	if _, err = tx.Exec(ctx, `UPDATE outbox_events SET lease_owner=$2,lease_until=now()+$3::interval,attempt_count=attempt_count+1 WHERE id=$1`, id, owner, lease.String()); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &event, nil
}

func (s *Store) RenewOutboxEventLease(ctx context.Context, id, owner string, lease time.Duration) (bool, error) {
	eventID, err := uuid.Parse(id)
	if err != nil || owner == "" || lease <= 0 {
		return false, errors.New("invalid outbox lease")
	}
	tag, err := s.AdminPool.Exec(ctx, `UPDATE outbox_events SET lease_until=now()+$3::interval WHERE id=$1 AND lease_owner=$2 AND processed_at IS NULL AND lease_until>=now()`, eventID, owner, lease.String())
	return err == nil && tag.RowsAffected() == 1, err
}

func (s *Store) FinishOutboxEvent(ctx context.Context, id, owner string, processingErr error) error {
	eventID, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	if processingErr == nil {
		var tag pgconn.CommandTag
		tag, err = s.AdminPool.Exec(ctx, `UPDATE outbox_events SET processed_at=now(),lease_owner=NULL,lease_until=NULL,last_error_code=NULL WHERE id=$1 AND lease_owner=$2 AND processed_at IS NULL AND lease_until>=now()`, eventID, owner)
		if err == nil && tag.RowsAffected() != 1 {
			err = errors.New("outbox lease lost")
		}
	} else {
		var tag pgconn.CommandTag
		tag, err = s.AdminPool.Exec(ctx, `UPDATE outbox_events SET available_at=now()+least(interval '5 minutes',interval '2 seconds'*power(2,least(attempt_count,7))),lease_owner=NULL,lease_until=NULL,last_error_code='PROJECTION_UNAVAILABLE' WHERE id=$1 AND lease_owner=$2 AND processed_at IS NULL AND lease_until>=now()`, eventID, owner)
		if err == nil && tag.RowsAffected() != 1 {
			err = errors.New("outbox lease lost")
		}
	}
	return err
}
