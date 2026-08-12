package server

import (
	"context"
	"fmt"
	"time"

	"diary-listener/backend/internal/rediscoord"
	"diary-listener/backend/internal/store"
	"github.com/google/uuid"
)

type aiEventProjectionBuilder struct {
	redis     *rediscoord.Client
	batchSize int
	lease     time.Duration
}

func (b *aiEventProjectionBuilder) Build(ctx context.Context, state store.AIEventReservationState) error {
	eventID := state.PublicID.String()
	stable := rediscoord.AIEventKeys(eventID, "")
	owner := uuid.NewString()
	locked, err := b.redis.AcquireAIEventBuildLock(ctx, stable.BuildLock, owner, b.lease)
	if err != nil || !locked {
		if err == nil {
			aiEventProjectionBuildAbandoned.Add(1)
		}
		return err
	}
	defer func() { _ = b.redis.ReleaseAIEventBuildLock(context.Background(), stable.BuildLock, owner) }()
	buildCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	leaseLost := make(chan struct{}, 1)
	go func() {
		ticker := time.NewTicker(max(time.Second, b.lease/3))
		defer ticker.Stop()
		for {
			select {
			case <-buildCtx.Done():
				return
			case <-ticker.C:
				ok, renewErr := b.redis.RenewAIEventBuildLock(buildCtx, stable.BuildLock, owner, b.lease)
				if renewErr != nil || !ok {
					select {
					case leaseLost <- struct{}{}:
					default:
					}
					cancel()
					return
				}
			}
		}
	}()

	started := time.Now()
	version := fmt.Sprintf("v%d", time.Now().UnixNano())
	keys := rediscoord.AIEventKeys(eventID, version)
	claimed := make([]string, 0, len(state.Tenants))
	for _, tenant := range state.Tenants {
		claimed = append(claimed, aiEventReservationMember(tenant))
	}
	eligible := make(map[string]int64, len(state.Eligible))
	for _, item := range state.Eligible {
		eligible[aiEventReservationMember(item.TenantID)] = item.Available
	}
	ttl := time.Until(state.ClosesAt) + 48*time.Hour
	if ttl <= 0 {
		ttl = time.Hour
	}
	if err = b.redis.BuildAIEventVersion(buildCtx, keys, state.OpensAt, state.ClosesAt, state.Remaining, state.PointsReward, claimed, eligible, ttl, b.batchSize); err != nil {
		aiEventProjectionBuildFailed.Add(1)
		_, _ = b.redis.CleanupAIEventVersion(context.Background(), keys, version)
		return err
	}
	select {
	case <-leaseLost:
		aiEventProjectionBuildAbandoned.Add(1)
		return fmt.Errorf("AI event projection build lease lost")
	default:
	}
	expected, ok, err := b.redis.Get(ctx, stable.ActiveVersion)
	if err != nil {
		return err
	}
	if !ok {
		expected = ""
	}
	switched, err := b.redis.SwitchAIEventVersion(ctx, stable, expected, version, owner)
	if err != nil || switched != 1 {
		aiEventProjectionBuildFailed.Add(1)
		_, _ = b.redis.CleanupAIEventVersion(context.Background(), keys, version)
		if err != nil {
			return err
		}
		return fmt.Errorf("AI event projection switch rejected: %d", switched)
	}
	aiEventProjectionBuildSuccess.Add(1)
	aiEventProjectionBuildDurationNanos.Store(uint64(time.Since(started)))
	return nil
}
