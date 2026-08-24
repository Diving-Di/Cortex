package main

import (
	"context"
	"cortex/backend/internal/config"
	"cortex/backend/internal/eventbus"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load configuration", "error", err)
		os.Exit(1)
	}
	if cfg.EventBus != "kafka" {
		slog.Error("EVENT_BUS must be kafka")
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	db, err := store.Open(ctx, cfg)
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	publisher := eventbus.NewKafkaREST(cfg.KafkaRESTURL)
	owner := "relay-" + uuid.NewString()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		event, err := db.ClaimOutboxEvent(ctx, "*", owner, 30*time.Second)
		if err == nil && event != nil {
			topic := topicFor(event.EventType)
			message := eventbus.Event{ID: event.ID, Type: event.EventType, AggregateID: event.AggregateID, SchemaVersion: 1, OccurredAt: event.OccurredAt}
			err = publisher.Publish(ctx, topic, event.AggregateID, message)
			_ = db.FinishOutboxEvent(ctx, event.ID, owner, err)
			continue
		} else if err != nil {
			slog.Error("outbox claim failed", "code", "OUTBOX_CLAIM_FAILED")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func topicFor(eventType string) string {
	switch {
	case len(eventType) >= 10 && eventType[:10] == "knowledge.":
		return "cortex.knowledge.index.v1"
	case len(eventType) >= 7 && eventType[:7] == "search.":
		return "cortex.search.projection.v1"
	default:
		return "cortex.audit.export.v1"
	}
}
