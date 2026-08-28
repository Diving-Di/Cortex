package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cortex/backend/internal/apierror"
	aieventsapp "cortex/backend/internal/application/aievents"
	"cortex/backend/internal/config"
	"cortex/backend/internal/domain"
	"cortex/backend/internal/httpx"
	"cortex/backend/internal/rediscoord"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

func (s *Server) aiPointBalance(w http.ResponseWriter, r *http.Request) {
	x, err := s.aiEvents.Balance(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, 200, x)
}
func (s *Server) currentAIEvent(w http.ResponseWriter, r *http.Request) {
	if !s.allowUserRequest(r, "ai-event-detail", 30, time.Minute) {
		httpx.WriteError(w, s.logger, apierror.New("RATE_LIMITED", "请求过于频繁", 429))
		return
	}
	if err := s.aiEvents.Ensure(r.Context(), time.Now()); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	x, err := s.aiEvents.Current(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, 200, x)
}
func (s *Server) aiEventPage(w http.ResponseWriter, r *http.Request) {
	if !s.allowUserRequest(r, "ai-event-page", 30, time.Minute) {
		httpx.WriteError(w, s.logger, apierror.New("RATE_LIMITED", "请求过于频繁", 429))
		return
	}
	if err := s.aiEvents.Ensure(r.Context(), time.Now()); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	x, err := s.aiEvents.Page(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, x)
}
func (s *Server) aiEventHistory(w http.ResponseWriter, r *http.Request) {
	x, err := s.aiEvents.History(r.Context())
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"items": x})
}
func (s *Server) getAIEvent(w http.ResponseWriter, r *http.Request) {
	id, err := aiEventPathID(r)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	x, err := s.aiEvents.Event(r.Context(), principalFrom(r.Context()), id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, x)
}
func aiEventPathID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(r.PathValue("eventID")))
	if err != nil {
		return uuid.Nil, apierror.Validation(nil)
	}
	return id, nil
}
func (s *Server) claimAIEvent(w http.ResponseWriter, r *http.Request) {
	totalStarted := time.Now()
	defer func() { observeAIEventStage(&aiEventClaimTotalNanos, &aiEventClaimTotalCount, totalStarted) }()
	rateStarted := time.Now()
	if !s.allowUserRequest(r, "ai-event-claim", 3, time.Minute) {
		observeAIEventStage(&aiEventClaimRateNanos, &aiEventClaimRateCount, rateStarted)
		aiEventClaimsError.Add(1)
		httpx.WriteError(w, s.logger, apierror.New("RATE_LIMITED", "领取请求过于频繁", 429))
		return
	}
	observeAIEventStage(&aiEventClaimRateNanos, &aiEventClaimRateCount, rateStarted)
	eventID, err := aiEventPathID(r)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	requestID, err := uuid.Parse(strings.TrimSpace(r.Header.Get("Idempotency-Key")))
	if err != nil {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	principal := principalFrom(r.Context())
	if s.claimRedis == nil {
		s.claimAIEventFallback(w, r, principal, eventID, requestID)
		return
	}
	member := aiEventReservationMember(principal.TenantID)
	redisStarted := time.Now()
	stable := rediscoord.AIEventKeys(eventID.String(), "")
	version, ok, redisErr := s.claimRedis.Get(r.Context(), stable.ActiveVersion)
	if redisErr == nil && !ok {
		redisErr = errors.New("AI event projection is not active")
	}
	reserved := -1
	var keys rediscoord.AIEventVersionKeys
	if redisErr == nil {
		keys = rediscoord.AIEventKeys(eventID.String(), version)
		reserved, redisErr = s.claimRedis.ReservePreparedVersioned(r.Context(), keys, version, member)
		if redisErr == nil && reserved == rediscoord.VersionChanged {
			aiEventProjectionVersionChanged.Add(1)
			if version, ok, redisErr = s.claimRedis.Get(r.Context(), stable.ActiveVersion); redisErr == nil && ok {
				keys = rediscoord.AIEventKeys(eventID.String(), version)
				reserved, redisErr = s.claimRedis.ReservePreparedVersioned(r.Context(), keys, version, member)
			}
		}
	}
	if redisErr != nil {
		observeAIEventStage(&aiEventClaimRedisNanos, &aiEventClaimRedisCount, redisStarted)
		aiEventClaimRedisErrors.Add(1)
		s.claimAIEventFallback(w, r, principal, eventID, requestID)
		return
	}
	observeAIEventStage(&aiEventClaimRedisNanos, &aiEventClaimRedisCount, redisStarted)
	if reserved == -1 {
		aiEventClaimsError.Add(1)
		httpx.WriteError(w, s.logger, apierror.New("AI_EVENT_UNAVAILABLE", "活动领取尚未就绪", 503))
		return
	}
	if reserved == rediscoord.VersionChanged {
		aiEventClaimsError.Add(1)
		httpx.WriteError(w, s.logger, apierror.New("AI_EVENT_BUSY", "活动投影正在切换，请稍后重试", 503))
		return
	}
	if reserved == -2 {
		aiEventClaimsError.Add(1)
		httpx.WriteError(w, s.logger, apierror.New("AI_EVENT_NOT_OPEN", "活动尚未开始", 409))
		return
	}
	if reserved == -3 {
		aiEventClaimsError.Add(1)
		httpx.WriteError(w, s.logger, apierror.New("AI_EVENT_CLOSED", "活动已结束", 409))
		return
	}
	if reserved == -4 {
		aiEventClaimsIneligible.Add(1)
		httpx.WriteError(w, s.logger, apierror.New("AI_EVENT_INELIGIBLE", "连续记录天数不足", 409))
		return
	}
	if reserved == 0 {
		aiEventClaimsSoldOut.Add(1)
		httpx.WriteError(w, s.logger, apierror.New("AI_EVENT_SOLD_OUT", "活动名额已领完", 409))
		return
	}
	if reserved == 2 {
		aiEventClaimsDuplicate.Add(1)
		x, e := s.aiEvents.MyClaim(r.Context(), principal, eventID)
		if e != nil {
			httpx.WriteError(w, s.logger, apierror.New("AI_EVENT_ALREADY_CLAIMED", "本场活动已经领取", 409))
			return
		}
		httpx.JSON(w, http.StatusOK, x)
		return
	}
	dbStarted := time.Now()
	queueCtx, queueCancel := context.WithTimeout(r.Context(), s.cfg.AIEventClaimQueueTimeout)
	defer queueCancel()
	select {
	case s.aiEventClaimSlots <- struct{}{}:
		defer func() { <-s.aiEventClaimSlots }()
	case <-queueCtx.Done():
		_ = s.claimRedis.Compensate(r.Context(), keys.Stock, keys.Claimed, keys.Window, keys.Points, keys.Pending, member)
		aiEventClaimCapacityBusy.Add(1)
		aiEventClaimsError.Add(1)
		httpx.WriteError(w, s.logger, apierror.New("AI_EVENT_BUSY", "领取请求繁忙，请稍后重试", 503))
		return
	}
	x, err := s.aiEvents.ClaimReserved(r.Context(), principal, eventID, requestID, version)
	observeAIEventStage(&aiEventClaimDBNanos, &aiEventClaimDBCount, dbStarted)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			aiEventClaimDBTimeouts.Add(1)
		}
		_ = s.claimRedis.Compensate(r.Context(), keys.Stock, keys.Claimed, keys.Window, keys.Points, keys.Pending, member)
		aiEventClaimsError.Add(1)
		httpx.WriteError(w, s.logger, err)
		return
	}
	confirmStarted := time.Now()
	_ = s.claimRedis.ConfirmReservation(r.Context(), keys.Pending, member)
	observeAIEventStage(&aiEventClaimConfirmNanos, &aiEventClaimConfirmCount, confirmStarted)
	aiEventClaimsReserved.Add(1)
	httpx.JSON(w, http.StatusOK, x)
}

func (s *Server) claimAIEventFallback(w http.ResponseWriter, r *http.Request, principal domain.Principal, eventID, requestID uuid.UUID) {
	s.aiEventBreakerMu.Lock()
	open := time.Now().Before(s.aiEventBreakerUntil)
	s.aiEventBreakerMu.Unlock()
	if open {
		aiEventClaimFallbackBusy.Add(1)
		aiEventClaimsError.Add(1)
		httpx.WriteError(w, s.logger, apierror.New("AI_EVENT_UNAVAILABLE", "活动领取暂不可用", 503))
		return
	}
	select {
	case s.aiEventFallbackSlots <- struct{}{}:
		defer func() { <-s.aiEventFallbackSlots }()
	default:
		aiEventClaimFallbackBusy.Add(1)
		httpx.WriteError(w, s.logger, apierror.New("AI_EVENT_BUSY", "领取请求繁忙，请稍后重试", 503))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
	defer cancel()
	x, err := s.aiEvents.ClaimFallback(ctx, principal, eventID, requestID)
	if err != nil {
		var appErr *apierror.Error
		business := errors.As(err, &appErr) && appErr.StatusCode == http.StatusConflict
		if !business {
			s.aiEventBreakerMu.Lock()
			s.aiEventBreakerFailures++
			if s.aiEventBreakerFailures >= 3 {
				s.aiEventBreakerUntil = time.Now().Add(15 * time.Second)
				s.aiEventBreakerFailures = 0
			}
			s.aiEventBreakerMu.Unlock()
		}
		aiEventClaimsError.Add(1)
		httpx.WriteError(w, s.logger, err)
		return
	}
	s.aiEventBreakerMu.Lock()
	s.aiEventBreakerFailures = 0
	s.aiEventBreakerMu.Unlock()
	// The database is authoritative. The next projection build repairs Redis.
	aiEventClaimsReserved.Add(1)
	httpx.JSON(w, http.StatusOK, x)
}
func aiEventReservationMember(tenantID uuid.UUID) string {
	digest := sha256.Sum256([]byte(tenantID.String()))
	return hex.EncodeToString(digest[:16])
}
func (s *Server) myAIEventClaim(w http.ResponseWriter, r *http.Request) {
	eventID, err := aiEventPathID(r)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	x, err := s.aiEvents.MyClaim(r.Context(), principalFrom(r.Context()), eventID)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, 200, x)
}
func (s *Server) getAIEventClaim(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("claimID")), 10, 64)
	if err != nil || id <= 0 {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	x, err := s.aiEvents.Claim(r.Context(), principalFrom(r.Context()), id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, 200, x)
}

func RunAIEventWorkers(ctx context.Context, cfg config.Config, db *store.Store, logger *slog.Logger) {
	events := aieventsapp.NewService(db)
	coord, _ := rediscoord.New(cfg.RedisURL)
	builder := &aiEventProjectionBuilder{redis: coord, batchSize: cfg.AIEventBuildBatchSize, lease: cfg.AIEventBuildLease}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			if err := events.Reconcile(ctx); err != nil && ctx.Err() == nil {
				logger.Error("reconcile AI event claimed slots", "error", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		var active *store.AIEventReservationState
		for {
			_ = events.Ensure(ctx, time.Now())
			now := time.Now()
			if active != nil && !now.Before(active.OpensAt) && now.Before(active.ClosesAt) && coord != nil {
				stable := rediscoord.AIEventKeys(active.PublicID.String(), "")
				if _, ok, err := coord.Get(ctx, stable.ActiveVersion); err == nil && ok {
					aiEventProjectionBuildSkippedOpen.Add(1)
					if err := events.SetReady(ctx, active.PublicID, true); err != nil {
						logger.Error("update AI event reservation readiness", "error", err)
					}
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						continue
					}
				}
			}
			if state, err := events.Reservation(ctx); err == nil {
				active = &state
				ready := false
				if coord != nil {
					if err := builder.Build(ctx, state); err != nil {
						logger.Error("build AI event projection", "error", err)
					} else {
						ready = true
					}
				}
				if err := events.SetReady(ctx, state.PublicID, ready); err != nil {
					logger.Error("update AI event reservation readiness", "error", err)
				}
			} else {
				logger.Error("read AI event reservation state", "error", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
