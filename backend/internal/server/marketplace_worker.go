package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	marketplaceapp "cortex/backend/internal/application/marketplace"
	"cortex/backend/internal/rediscoord"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

func RunMarketplaceWorker(ctx context.Context, db *store.Store, redis *rediscoord.Client, logger *slog.Logger) {
	service := marketplaceapp.NewService(db)
	if redis == nil {
		return
	}
	rebuiltAt := time.Now()
	if rows, err := service.Rankings(ctx); err != nil {
		logger.Error("read template ranking projections", "error", err)
	} else {
		items := make([]rediscoord.RankingProjection, 0, len(rows))
		for _, row := range rows {
			items = append(items, rediscoord.RankingProjection{PublicID: row.PublicID, PublishedAt: row.PublishedAt, TrendingScore: row.TrendingScore})
		}
		if err := redis.RebuildTemplateRankings(ctx, items); err != nil {
			logger.Error("rebuild template rankings", "error", err)
		} else if err := service.MarkRebuilt(ctx, rebuiltAt); err != nil {
			logger.Error("mark rebuilt template outbox", "error", err)
		}
	}
	go func() {
		owner := uuid.NewString()
		lease := 30 * time.Second
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			event, err := service.ClaimEvent(ctx, "template", owner, lease)
			if err != nil {
				logger.Error("claim marketplace outbox", "error", err)
				continue
			}
			if event == nil {
				continue
			}
			leaseCtx, cancelLease := context.WithCancel(ctx)
			leaseLost := make(chan struct{}, 1)
			go func(id string) {
				ticker := time.NewTicker(lease / 3)
				defer ticker.Stop()
				for {
					select {
					case <-leaseCtx.Done():
						return
					case <-ticker.C:
						ok, renewErr := service.RenewEvent(leaseCtx, id, owner, lease)
						if renewErr != nil || !ok {
							templateOutboxLeaseLost.Add(1)
							select {
							case leaseLost <- struct{}{}:
							default:
							}
							return
						}
						templateOutboxLeaseRenewed.Add(1)
					}
				}
			}(event.ID)
			if event.EventType == "template.withdrawn" || event.EventType == "template.deleted" {
				err = redis.DeleteTemplateProjections(ctx, event.AggregateID)
			} else {
				payload := struct {
					Delta   int64  `json:"delta"`
					Visitor string `json:"visitor"`
				}{Delta: 1}
				_ = json.Unmarshal(event.Payload, &payload)
				_ = redis.Delete(ctx, "cortex:tpl:detail:"+event.AggregateID)
				projection, projectionErr := service.Projection(ctx, event.AggregateID)
				if projectionErr != nil {
					err = projectionErr
				} else if !projection.Published {
					err = redis.DeleteTemplateProjections(ctx, event.AggregateID)
				} else {
					err = redis.ApplyTemplateProjection(ctx, event.ID, event.AggregateID, event.EventType, payload.Visitor, projection.PublishedAt, projection.TrendingScore, projection.DailyScore)
				}
			}
			cancelLease()
			select {
			case <-leaseLost:
				err = errors.New("outbox lease lost")
			default:
			}
			if finishErr := service.FinishEvent(ctx, event.ID, owner, err); finishErr != nil {
				if strings.Contains(finishErr.Error(), "lease lost") {
					templateOutboxFinishFenced.Add(1)
				}
				logger.Error("finish marketplace outbox", "error", finishErr)
			}
		}
	}()
}
