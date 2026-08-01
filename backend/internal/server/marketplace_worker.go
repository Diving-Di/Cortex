package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"diary-listener/backend/internal/rediscoord"
	"diary-listener/backend/internal/store"
	"github.com/google/uuid"
)

func RunMarketplaceWorker(ctx context.Context, db *store.Store, redis *rediscoord.Client, logger *slog.Logger) {
	if redis == nil {
		return
	}
	rebuiltAt := time.Now()
	if rows, err := db.ListTemplateRankingProjections(ctx); err != nil {
		logger.Error("read template ranking projections", "error", err)
	} else {
		items := make([]rediscoord.RankingProjection, 0, len(rows))
		for _, row := range rows {
			items = append(items, rediscoord.RankingProjection{PublicID: row.PublicID, PublishedAt: row.PublishedAt, TrendingScore: row.TrendingScore})
		}
		if err := redis.RebuildTemplateRankings(ctx, items); err != nil {
			logger.Error("rebuild template rankings", "error", err)
		} else if err := db.MarkTemplateOutboxRebuilt(ctx, rebuiltAt); err != nil {
			logger.Error("mark rebuilt template outbox", "error", err)
		}
	}
	go func() {
		owner := uuid.NewString()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			event, err := db.ClaimOutboxEvent(ctx, owner)
			if err != nil {
				logger.Error("claim marketplace outbox", "error", err)
				continue
			}
			if event == nil {
				continue
			}
			if event.AggregateType != "template" {
				err = nil
			} else if event.EventType == "template.withdrawn" || event.EventType == "template.deleted" {
				err = redis.DeleteTemplateProjections(ctx, event.AggregateID)
			} else {
				payload := struct {
					Delta   int64  `json:"delta"`
					Visitor string `json:"visitor"`
				}{Delta: 1}
				_ = json.Unmarshal(event.Payload, &payload)
				_ = redis.Delete(ctx, "diary:tpl:detail:"+event.AggregateID)
				projection, projectionErr := db.GetTemplateEventProjection(ctx, event.AggregateID)
				if projectionErr != nil {
					err = projectionErr
				} else if !projection.Published {
					err = redis.DeleteTemplateProjections(ctx, event.AggregateID)
				} else {
					err = redis.ApplyTemplateProjection(ctx, event.ID, event.AggregateID, event.EventType, payload.Visitor, projection.PublishedAt, projection.TrendingScore, projection.DailyScore)
				}
			}
			if finishErr := db.FinishOutboxEvent(ctx, event.ID, owner, err); finishErr != nil {
				logger.Error("finish marketplace outbox", "error", finishErr)
			}
		}
	}()
}
