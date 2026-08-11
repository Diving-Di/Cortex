package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/config"
	"diary-listener/backend/internal/httpx"
	"diary-listener/backend/internal/rediscoord"
	"diary-listener/backend/internal/store"
	"github.com/google/uuid"
)

func (s *Server) aiPointBalance(w http.ResponseWriter, r *http.Request) {
	x, err := s.store.GetAIPointBalance(r.Context(), principalFrom(r.Context()))
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
	if err := s.store.EnsureDailyAIEvent(r.Context(), time.Now()); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	x, err := s.store.GetCurrentAIEvent(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, 200, x)
}
func (s *Server) aiEventHistory(w http.ResponseWriter, r *http.Request) {
	x, err := s.store.ListAIEventHistory(r.Context())
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
	x, err := s.store.GetAIEvent(r.Context(), principalFrom(r.Context()), id)
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
	if !s.allowUserRequest(r, "ai-event-claim", 3, time.Minute) || !s.allowIPRequest(r, "ai-event-claim", 30, time.Minute) {
		aiEventClaimsError.Add(1)
		httpx.WriteError(w, s.logger, apierror.New("RATE_LIMITED", "领取请求过于频繁", 429))
		return
	}
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
	if s.redis == nil {
		aiEventClaimsError.Add(1)
		httpx.WriteError(w, s.logger, apierror.New("AI_EVENT_UNAVAILABLE", "活动领取暂不可用", 503))
		return
	}
	stockKey := "diary:ai-event:{" + eventID.String() + "}:stock"
	claimedKey := "diary:ai-event:{" + eventID.String() + "}:claimed"
	windowKey := "diary:ai-event:{" + eventID.String() + "}:window"
	eligibleKey := "diary:ai-event:{" + eventID.String() + "}:eligible"
	pointsKey := "diary:ai-event:{" + eventID.String() + "}:points"
	pendingKey := "diary:ai-event:{" + eventID.String() + "}:pending"
	member := aiEventReservationMember(principal.TenantID)
	reserved, redisErr := s.redis.ReservePrepared(r.Context(), stockKey, claimedKey, windowKey, eligibleKey, pointsKey, pendingKey, member)
	if redisErr != nil {
		aiEventClaimsError.Add(1)
		httpx.WriteError(w, s.logger, apierror.New("AI_EVENT_UNAVAILABLE", "活动领取暂不可用", 503))
		return
	}
	if reserved == -1 {
		aiEventClaimsError.Add(1)
		httpx.WriteError(w, s.logger, apierror.New("AI_EVENT_UNAVAILABLE", "活动领取尚未就绪", 503))
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
		x, e := s.store.GetMyAIEventClaim(r.Context(), principal, eventID)
		if e != nil {
			httpx.WriteError(w, s.logger, apierror.New("AI_EVENT_ALREADY_CLAIMED", "本场活动已经领取", 409))
			return
		}
		httpx.JSON(w, http.StatusOK, x)
		return
	}
	x, err := s.store.ClaimAIEvent(r.Context(), principal, eventID, requestID)
	if err != nil {
		_ = s.redis.Compensate(r.Context(), stockKey, claimedKey, windowKey, pointsKey, pendingKey, member)
		aiEventClaimsError.Add(1)
		httpx.WriteError(w, s.logger, err)
		return
	}
	_ = s.redis.ConfirmReservation(r.Context(), pendingKey, member)
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
	x, err := s.store.GetMyAIEventClaim(r.Context(), principalFrom(r.Context()), eventID)
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
	x, err := s.store.GetAIEventClaim(r.Context(), principalFrom(r.Context()), id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, 200, x)
}

func RunAIEventWorkers(ctx context.Context, cfg config.Config, db *store.Store, logger *slog.Logger) {
	coord, _ := rediscoord.New(cfg.RedisURL)
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			_ = db.EnsureDailyAIEvent(ctx, time.Now())
			if state, err := db.GetAIEventReservationState(ctx); err == nil {
				ready := false
				if coord != nil {
					members := make([]string, 0, len(state.Tenants))
					for _, tenant := range state.Tenants {
						members = append(members, aiEventReservationMember(tenant))
					}
					ttl := time.Until(state.ClosesAt) + 48*time.Hour
					if ttl > 0 {
						stock := "diary:ai-event:{" + state.PublicID.String() + "}:stock"
						claimed := "diary:ai-event:{" + state.PublicID.String() + "}:claimed"
						window := "diary:ai-event:{" + state.PublicID.String() + "}:window"
						eligibleKey := "diary:ai-event:{" + state.PublicID.String() + "}:eligible"
						pointsKey := "diary:ai-event:{" + state.PublicID.String() + "}:points"
						pendingKey := "diary:ai-event:{" + state.PublicID.String() + "}:pending"
						eligible := make(map[string]int64, len(state.Eligible))
						for _, item := range state.Eligible {
							eligible[aiEventReservationMember(item.TenantID)] = item.Available
						}
						if err := coord.WarmEvent(ctx, stock, claimed, window, eligibleKey, pointsKey, pendingKey, state.OpensAt, state.ClosesAt, state.Remaining, state.PointsReward, members, eligible, ttl); err != nil {
							logger.Error("warm AI event reservation", "error", err)
						} else {
							ready = true
						}
					}
				}
				if err := db.SetAIEventReservationReady(ctx, state.PublicID, ready); err != nil {
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
