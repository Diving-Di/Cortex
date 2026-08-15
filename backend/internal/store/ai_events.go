package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type AIFlashEvent struct {
	PublicID            uuid.UUID `json:"id"`
	EventDate           time.Time `json:"event_date"`
	Timezone            string    `json:"timezone"`
	OpensAt             time.Time `json:"opens_at"`
	ClosesAt            time.Time `json:"closes_at"`
	TotalSlots          int       `json:"total_slots"`
	RemainingSlots      int       `json:"remaining_slots"`
	PointsReward        int64     `json:"points_reward"`
	RequiredStreakDays  int       `json:"required_streak_days"`
	Status              string    `json:"status"`
	ServerTime          time.Time `json:"server_time"`
	Eligible            bool      `json:"eligible"`
	StreakDays          int       `json:"streak_days"`
	Claimed             bool      `json:"claimed"`
	ShowDashboardPrompt bool      `json:"show_dashboard_prompt"`
}
type AIPointBalance struct {
	PeriodStart time.Time `json:"period_start"`
	Granted     int64     `json:"granted"`
	Consumed    int64     `json:"consumed"`
	Held        int64     `json:"held"`
	Available   int64     `json:"available"`
	Version     int       `json:"version"`
}
type AIFlashClaim struct {
	ID           int64      `json:"id"`
	EventID      uuid.UUID  `json:"event_id"`
	Status       string     `json:"status"`
	PointsReward int64      `json:"points_reward"`
	StreakDays   int        `json:"streak_days"`
	ClaimedAt    time.Time  `json:"claimed_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	ReportNoteID *int32     `json:"report_note_id,omitempty"`
	ErrorCode    *string    `json:"error_code,omitempty"`
}
type AIEventHistoryItem struct {
	DisplayName string    `json:"display_name"`
	ClaimedAt   time.Time `json:"claimed_at"`
}
type AIEventReservationState struct {
	PublicID          uuid.UUID
	OpensAt, ClosesAt time.Time
	PointsReward      int64
	Remaining         int
	Tenants           []uuid.UUID
	Eligible          []AIEventEligibleTenant
}

type AIEventEligibleTenant struct {
	TenantID  uuid.UUID
	Available int64
}

func (s *Store) GetAIEventReservationState(ctx context.Context) (AIEventReservationState, error) {
	var x AIEventReservationState
	var eventID int64
	var total, claimed, required int
	var eventDate time.Time
	var timezone string
	var monthlyGrant int64
	err := s.AdminPool.QueryRow(ctx, `SELECT e.id,e.public_id,e.event_date,e.timezone,e.opens_at,e.closes_at,e.total_slots,e.claimed_slots,e.points_cost,e.required_streak_days,s.monthly_grant_points FROM ai_flash_events e JOIN ai_flash_event_settings s ON s.id=1 WHERE e.event_date >= (now() AT TIME ZONE e.timezone)::date AND e.status NOT IN('paused','cancelled') ORDER BY e.event_date LIMIT 1`).Scan(&eventID, &x.PublicID, &eventDate, &timezone, &x.OpensAt, &x.ClosesAt, &total, &claimed, &x.PointsReward, &required, &monthlyGrant)
	if err != nil {
		return x, err
	}
	x.Remaining = max(0, total-claimed)
	statsTx, err := s.AdminPool.Begin(ctx)
	if err != nil {
		return x, err
	}
	defer func() { _ = statsTx.Rollback(ctx) }()
	if _, err = statsTx.Exec(ctx, `DELETE FROM tenant_daily_writing_stats WHERE timezone=$1 AND local_date BETWEEN $2::date-($3::int-1) AND $2::date`, timezone, eventDate, required); err != nil {
		return x, err
	}
	if _, err = statsTx.Exec(ctx, `INSERT INTO tenant_daily_writing_stats(tenant_id,local_date,timezone,eligible_note_count,eligible_word_count)
		SELECT tenant_id,local_date,$1,count(*)::int,sum(char_length(btrim(content)))::bigint FROM (
			SELECT tenant_id,COALESCE(note_date,(created_at AT TIME ZONE $1)::date) local_date,content,created_at FROM notes
			WHERE deleted_at IS NULL AND type IN('normal','daily') AND char_length(btrim(content))>=50
		) n WHERE local_date BETWEEN $2::date-($3::int-1) AND $2::date AND (local_date<>$2::date OR created_at<$4)
		GROUP BY tenant_id,local_date`, timezone, eventDate, required, x.OpensAt); err != nil {
		return x, err
	}
	if err = statsTx.Commit(ctx); err != nil {
		return x, err
	}
	eligibleRows, err := s.AdminPool.Query(ctx, `SELECT t.id,CASE
		WHEN a.tenant_id IS NULL THEN $3::bigint
		WHEN to_char(a.period_start,'YYYY-MM')=to_char($1::date,'YYYY-MM') THEN GREATEST(0,a.granted_points-a.consumed_points-a.held_points)
		WHEN a.held_points=0 THEN $3::bigint ELSE 0 END
		FROM tenants t LEFT JOIN ai_point_accounts a ON a.tenant_id=t.id
		WHERE t.status='active' AND t.deleted_at IS NULL AND (SELECT count(*) FROM tenant_daily_writing_stats d WHERE d.tenant_id=t.id AND d.timezone=$2 AND d.local_date BETWEEN $1::date-($4::int-1) AND $1::date AND d.eligible_note_count>0)=$4`, eventDate, timezone, monthlyGrant, required)
	if err != nil {
		return x, err
	}
	for eligibleRows.Next() {
		var item AIEventEligibleTenant
		if err := eligibleRows.Scan(&item.TenantID, &item.Available); err != nil {
			eligibleRows.Close()
			return x, err
		}
		x.Eligible = append(x.Eligible, item)
	}
	eligibleRows.Close()
	if err := eligibleRows.Err(); err != nil {
		return x, err
	}
	rows, err := s.AdminPool.Query(ctx, `SELECT tenant_id FROM ai_flash_claims WHERE event_id=$1`, eventID)
	if err != nil {
		return x, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return x, err
		}
		x.Tenants = append(x.Tenants, id)
	}
	return x, rows.Err()
}

func (s *Store) SetAIEventReservationReady(ctx context.Context, publicID uuid.UUID, ready bool) error {
	_, err := s.AdminPool.Exec(ctx, `UPDATE ai_flash_events SET reservation_ready=$2,updated_at=now() WHERE public_id=$1 AND status NOT IN('cancelled')`, publicID, ready)
	return err
}

func (s *Store) ListAIEventHistory(ctx context.Context) ([]AIEventHistoryItem, error) {
	rows, err := s.AdminPool.Query(ctx, `SELECT id,date_trunc('minute',claimed_at) FROM ai_flash_claims WHERE status='succeeded' AND claimed_at>=now()-interval '7 days' ORDER BY claimed_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []AIEventHistoryItem
	for rows.Next() {
		var id int64
		var x AIEventHistoryItem
		if err := rows.Scan(&id, &x.ClaimedAt); err != nil {
			return nil, err
		}
		digest := sha256.Sum256([]byte(fmt.Sprint(id)))
		x.DisplayName = fmt.Sprintf("记录者·%X", digest[:2])
		result = append(result, x)
	}
	return result, rows.Err()
}

func (s *Store) EnsureDailyAIEvent(ctx context.Context, now time.Time) error {
	var timezone string
	var hour, minute, duration, slots, points, streak int
	var enabled bool
	if err := s.AdminPool.QueryRow(ctx, `SELECT timezone,open_hour,open_minute,duration_minutes,total_slots,points_cost,required_streak_days,enabled FROM ai_flash_event_settings WHERE id=1`).Scan(&timezone, &hour, &minute, &duration, &slots, &points, &streak, &enabled); err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	date, opens, closes := configuredAIEventWindow(now, timezone, hour, minute, duration)
	_, err := s.AdminPool.Exec(ctx, `INSERT INTO ai_flash_events(public_id,event_date,timezone,opens_at,closes_at,total_slots,points_cost,required_streak_days) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(event_date,timezone) DO NOTHING`, uuid.New(), date, timezone, opens.UTC(), closes.UTC(), slots, points, streak)
	return err
}
func configuredAIEventWindow(now time.Time, timezone string, hour, minute, duration int) (time.Time, time.Time, time.Time) {
	zone, err := time.LoadLocation(timezone)
	if err != nil {
		zone = time.UTC
	}
	local := now.In(zone)
	date := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	opens := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, zone)
	return date, opens, opens.Add(time.Duration(duration) * time.Minute)
}

func ensurePointAccount(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, now time.Time) (AIPointBalance, error) {
	var monthlyGrant int64
	var timezone string
	if err := tx.QueryRow(ctx, `SELECT monthly_grant_points,timezone FROM ai_flash_event_settings WHERE id=1`).Scan(&monthlyGrant, &timezone); err != nil {
		return AIPointBalance{}, err
	}
	zone, err := time.LoadLocation(timezone)
	if err != nil {
		return AIPointBalance{}, err
	}
	local := now.In(zone)
	period := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, zone)
	var x AIPointBalance
	err = tx.QueryRow(ctx, `INSERT INTO ai_point_accounts(tenant_id,period_start,granted_points) VALUES($1,$2,$3) ON CONFLICT(tenant_id) DO UPDATE SET period_start=EXCLUDED.period_start,granted_points=EXCLUDED.granted_points,consumed_points=0,held_points=0,version=ai_point_accounts.version+1,updated_at=now() WHERE ai_point_accounts.period_start<>EXCLUDED.period_start AND ai_point_accounts.held_points=0 RETURNING period_start,granted_points,consumed_points,held_points,granted_points-consumed_points-held_points,version`, tenant, period, monthlyGrant).Scan(&x.PeriodStart, &x.Granted, &x.Consumed, &x.Held, &x.Available, &x.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT period_start,granted_points,consumed_points,held_points,granted_points-consumed_points-held_points,version FROM ai_point_accounts WHERE tenant_id=$1`, tenant).Scan(&x.PeriodStart, &x.Granted, &x.Consumed, &x.Held, &x.Available, &x.Version)
	}
	if err == nil && x.PeriodStart.Format("2006-01") != period.Format("2006-01") {
		return AIPointBalance{}, apierror.New("AI_POINTS_PERIOD_ROLLOVER_PENDING", "上月生成任务尚未结算", 409)
	}
	if err == nil {
		grantID := uuid.NewSHA1(tenant, []byte("grant:"+x.PeriodStart.Format(time.DateOnly)))
		_, err = tx.Exec(ctx, `INSERT INTO ai_point_ledger(tenant_id,period_start,event_id,entry_type,points,reference_type,reference_id) VALUES($1,$2,$3,'grant',$4,'monthly_grant',$5) ON CONFLICT DO NOTHING`, tenant, x.PeriodStart, grantID, x.Granted, x.PeriodStart.Format("2006-01"))
	}
	return x, err
}
func (s *Store) GetAIPointBalance(ctx context.Context, p domain.Principal) (AIPointBalance, error) {
	var x AIPointBalance
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		var err error
		x, err = ensurePointAccount(ctx, tx, p.TenantID, time.Now())
		return err
	})
	return x, err
}

func aiEventStreakDays(ctx context.Context, tx pgx.Tx, p domain.Principal, eventDate time.Time, timezone string, opensAt time.Time, required int) (int, error) {
	var count int
	err := tx.QueryRow(ctx, `WITH days AS (SELECT generate_series($2::date-($3::int-1),$2::date,'1 day')::date d), valid AS (SELECT DISTINCT COALESCE(note_date,(created_at AT TIME ZONE $4)::date) d FROM notes WHERE tenant_id=$1 AND deleted_at IS NULL AND type IN('normal','daily') AND char_length(btrim(content))>=50 AND COALESCE(note_date,(created_at AT TIME ZONE $4)::date) BETWEEN $2::date-($3::int-1) AND $2::date AND (COALESCE(note_date,(created_at AT TIME ZONE $4)::date)<>$2::date OR created_at < $5)) SELECT count(*) FROM days JOIN valid USING(d)`, p.TenantID, eventDate, required, timezone, opensAt).Scan(&count)
	return count, err
}
func (s *Store) GetCurrentAIEvent(ctx context.Context, p domain.Principal) (AIFlashEvent, error) {
	return s.getAIEvent(ctx, p, nil)
}

func (s *Store) GetAIEvent(ctx context.Context, p domain.Principal, publicID uuid.UUID) (AIFlashEvent, error) {
	return s.getAIEvent(ctx, p, &publicID)
}

func (s *Store) getAIEvent(ctx context.Context, p domain.Principal, publicID *uuid.UUID) (AIFlashEvent, error) {
	var x AIFlashEvent
	now := time.Now()
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		var eventID int64
		var claimedCount int
		var reservationReady bool
		err := tx.QueryRow(ctx, `SELECT id,public_id,event_date,timezone,opens_at,closes_at,total_slots,points_cost,required_streak_days,status,reservation_ready,claimed_slots,EXISTS(SELECT 1 FROM ai_flash_claims WHERE event_id=e.id AND tenant_id=$1) FROM ai_flash_events e WHERE ($2::uuid IS NOT NULL AND public_id=$2) OR ($2::uuid IS NULL AND event_date >= (now() AT TIME ZONE timezone)::date) ORDER BY event_date LIMIT 1`, p.TenantID, publicID).Scan(&eventID, &x.PublicID, &x.EventDate, &x.Timezone, &x.OpensAt, &x.ClosesAt, &x.TotalSlots, &x.PointsReward, &x.RequiredStreakDays, &x.Status, &reservationReady, &claimedCount, &x.Claimed)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("AI_EVENT_NOT_FOUND", "暂无活动", 404)
		} else if err != nil {
			return err
		}
		x.RemainingSlots = max(0, x.TotalSlots-claimedCount)
		x.ServerTime = now
		if !reservationReady {
			x.Status = "paused"
		} else if x.Status == "scheduled" {
			if !now.Before(x.ClosesAt) {
				x.Status = "closed"
			} else if !now.Before(x.OpensAt) {
				x.Status = "open"
			}
		}
		x.ShowDashboardPrompt = !x.Claimed && !now.Before(x.OpensAt.Add(-30*time.Minute)) && now.Before(x.ClosesAt)
		x.StreakDays, err = aiEventStreakDays(ctx, tx, p, x.EventDate, x.Timezone, x.OpensAt, x.RequiredStreakDays)
		x.Eligible = err == nil && x.StreakDays >= x.RequiredStreakDays
		return err
	})
	return x, err
}

func (s *Store) ClaimAIEvent(ctx context.Context, p domain.Principal, publicID, requestID uuid.UUID) (AIFlashClaim, error) {
	return s.claimAIEvent(ctx, p, publicID, requestID, false, "")
}

func (s *Store) ClaimAIEventFallback(ctx context.Context, p domain.Principal, publicID, requestID uuid.UUID) (AIFlashClaim, error) {
	return s.claimAIEvent(ctx, p, publicID, requestID, true, "fallback")
}

func (s *Store) ClaimAIEventReserved(ctx context.Context, p domain.Principal, publicID, requestID uuid.UUID, projectionVersion string) (AIFlashClaim, error) {
	return s.claimAIEvent(ctx, p, publicID, requestID, false, projectionVersion)
}

func (s *Store) claimAIEvent(ctx context.Context, p domain.Principal, publicID, requestID uuid.UUID, allowUnready bool, projectionVersion string) (AIFlashClaim, error) {
	var result AIFlashClaim
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		var eventID int64
		var date time.Time
		var zone, eventStatus string
		var reservationReady bool
		var opens, closes time.Time
		var slots, required int
		var cost int64
		if err := tx.QueryRow(ctx, `SELECT id,event_date,timezone,opens_at,closes_at,total_slots,points_cost,required_streak_days,status,reservation_ready FROM ai_flash_events WHERE public_id=$1`, publicID).Scan(&eventID, &date, &zone, &opens, &closes, &slots, &cost, &required, &eventStatus, &reservationReady); errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("AI_EVENT_NOT_FOUND", "活动不存在", 404)
		} else if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT id,status,points_cost,streak_days_at_claim,claimed_at,finished_at,report_note_id,error_code FROM ai_flash_claims WHERE event_id=$1 AND tenant_id=$2`, eventID, p.TenantID).Scan(&result.ID, &result.Status, &result.PointsReward, &result.StreakDays, &result.ClaimedAt, &result.FinishedAt, &result.ReportNoteID, &result.ErrorCode); err == nil {
			result.EventID = publicID
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		now := time.Now()
		if (!reservationReady && !allowUnready) || eventStatus == "paused" || eventStatus == "cancelled" {
			return apierror.New("AI_EVENT_UNAVAILABLE", "活动暂不可用", 503)
		}
		if now.Before(opens) {
			return apierror.New("AI_EVENT_NOT_OPEN", "活动尚未开始", 409)
		}
		if !now.Before(closes) {
			return apierror.New("AI_EVENT_CLOSED", "活动已结束", 409)
		}
		var reusedRequest bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ai_flash_claims WHERE tenant_id=$1 AND request_id=$2)`, p.TenantID, requestID).Scan(&reusedRequest); err != nil {
			return err
		}
		if reusedRequest {
			return apierror.New("IDEMPOTENCY_KEY_REUSED", "幂等键已用于其他活动", 409)
		}
		streak, err := aiEventStreakDays(ctx, tx, p, date, zone, opens, required)
		if err != nil {
			return err
		}
		if streak < required {
			return apierror.New("AI_EVENT_INELIGIBLE", "连续记录天数不足", 409)
		}
		var claimedSlots int
		if err := tx.QueryRow(ctx, `UPDATE ai_flash_events SET claimed_slots=claimed_slots+1,updated_at=now() WHERE id=$1 AND claimed_slots<total_slots RETURNING claimed_slots`, eventID).Scan(&claimedSlots); errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("AI_EVENT_SOLD_OUT", "活动名额已领完", 409)
		} else if err != nil {
			return err
		}
		balance, err := ensurePointAccount(ctx, tx, p.TenantID, now)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE ai_point_accounts SET granted_points=granted_points+$1,version=version+1,updated_at=now() WHERE tenant_id=$2`, cost, p.TenantID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO ai_point_ledger(tenant_id,period_start,event_id,entry_type,points,reference_type,reference_id) VALUES($1,$2,$3,'grant',$4,'ai_flash_event_reward',$5)`, p.TenantID, balance.PeriodStart, requestID, cost, publicID.String()); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO ai_flash_claims(event_id,tenant_id,user_id,request_id,status,points_cost,streak_days_at_claim,finished_at,reservation_token,reservation_version) VALUES($1,$2,$3,$4,'succeeded',$5,$6,now(),$4,$7) RETURNING id,status,points_cost,streak_days_at_claim,claimed_at,finished_at,report_note_id,error_code`, eventID, p.TenantID, p.UserID, requestID, cost, streak, projectionVersion).Scan(&result.ID, &result.Status, &result.PointsReward, &result.StreakDays, &result.ClaimedAt, &result.FinishedAt, &result.ReportNoteID, &result.ErrorCode); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO ai_event_reservations(token,event_id,tenant_id,projection_version,state,resolved_at) VALUES($1,$2,$3,$4,'confirmed',now()) ON CONFLICT(token) DO UPDATE SET state='confirmed',resolved_at=now()`, requestID, eventID, p.TenantID, projectionVersion); err != nil {
			return err
		}
		result.EventID = publicID
		_, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload) VALUES($1,'ai_flash_claim',$2,'ai_flash_claim.succeeded','{}')`, uuid.New(), fmt.Sprint(result.ID))
		return err
	})
	return result, err
}

func (s *Store) GetMyAIEventClaim(ctx context.Context, p domain.Principal, publicID uuid.UUID) (AIFlashClaim, error) {
	var x AIFlashClaim
	x.EventID = publicID
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT c.id,c.status,c.points_cost,c.streak_days_at_claim,c.claimed_at,c.finished_at,c.report_note_id,c.error_code FROM ai_flash_claims c JOIN ai_flash_events e ON e.id=c.event_id WHERE c.tenant_id=$1 AND e.public_id=$2`, p.TenantID, publicID).Scan(&x.ID, &x.Status, &x.PointsReward, &x.StreakDays, &x.ClaimedAt, &x.FinishedAt, &x.ReportNoteID, &x.ErrorCode)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("AI_EVENT_CLAIM_NOT_FOUND", "尚未领取", 404)
		}
		return err
	})
	return x, err
}
func (s *Store) GetAIEventClaim(ctx context.Context, p domain.Principal, claimID int64) (AIFlashClaim, error) {
	var x AIFlashClaim
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT c.id,e.public_id,c.status,c.points_cost,c.streak_days_at_claim,c.claimed_at,c.finished_at,c.report_note_id,c.error_code FROM ai_flash_claims c JOIN ai_flash_events e ON e.id=c.event_id WHERE c.tenant_id=$1 AND c.id=$2`, p.TenantID, claimID).Scan(&x.ID, &x.EventID, &x.Status, &x.PointsReward, &x.StreakDays, &x.ClaimedAt, &x.FinishedAt, &x.ReportNoteID, &x.ErrorCode)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("AI_EVENT_CLAIM_NOT_FOUND", "领取记录不存在", 404)
		}
		return err
	})
	return x, err
}
