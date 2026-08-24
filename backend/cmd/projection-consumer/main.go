package main

import (
	"context"
	"cortex/backend/internal/config"
	"cortex/backend/internal/eventbus"
	"cortex/backend/internal/searchindex"
	"cortex/backend/internal/store"
	"crypto/sha256"
	"encoding/hex"
	"github.com/google/uuid"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const group = "cortex-search-projection-v1"

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load configuration", "error", err)
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
	es := searchindex.New(cfg.ElasticsearchURLs, cfg.ElasticsearchUsername, cfg.ElasticsearchPassword, cfg.ElasticsearchIndexAlias)
	if err = es.EnsureIndex(ctx); err != nil {
		slog.Error("ensure search index", "error", err)
		os.Exit(1)
	}
	for ctx.Err() == nil {
		consumer, connectErr := eventbus.NewConsumer(ctx, cfg.KafkaRESTURL, group, []string{"cortex.search.projection.v1"})
		if connectErr != nil {
			slog.Error("create kafka consumer", "code", "KAFKA_CONSUMER_UNAVAILABLE")
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
				continue
			}
		}
		for ctx.Err() == nil {
			records, pollErr := consumer.Poll(ctx)
			if pollErr != nil {
				slog.Error("poll kafka", "code", "KAFKA_POLL_FAILED")
				break
			}
			for _, record := range records {
				process(ctx, db, es, record)
			}
			if commitErr := consumer.Commit(ctx); commitErr != nil {
				slog.Error("commit kafka", "code", "KAFKA_COMMIT_FAILED")
				break
			}
		}
	}
}
func process(ctx context.Context, db *store.Store, es *searchindex.Elasticsearch, r eventbus.Record) {
	eventID, e1 := uuid.Parse(r.Value.ID)
	documentID, e2 := uuid.Parse(r.Value.AggregateID)
	if e1 != nil || e2 != nil {
		return
	}
	fresh, err := db.ConsumerReceived(ctx, group, eventID)
	if err != nil || !fresh {
		return
	}
	chunks, err := db.LoadSearchProjection(ctx, documentID)
	if err != nil || len(chunks) == 0 {
		_ = db.DeadLetter(ctx, group, r.Topic, eventID, "PROJECTION_SOURCE_UNAVAILABLE", 1)
		_ = db.FinishConsumerReceipt(ctx, group, eventID, "failed")
		return
	}
	docs := make([]searchindex.Document, len(chunks))
	hash := sha256.New()
	for i, c := range chunks {
		var collection *string
		if c.CollectionID != nil {
			x := c.CollectionID.String()
			collection = &x
		}
		docs[i] = searchindex.Document{ID: c.ChunkID.String(), TenantID: c.TenantID.String(), DocumentID: c.DocumentID.String(), ParentID: c.ParentID.String(), IndexVersion: c.IndexVersion, CollectionID: collection, Title: c.Title, SourceType: c.SourceType, SourcePath: c.SourcePath, Content: c.Content, SearchText: c.SearchText, Heading: c.Heading, Embedding: c.Embedding}
		hash.Write([]byte(docs[i].ID))
	}
	err = es.BulkUpsert(ctx, docs)
	_ = db.CompleteSearchProjection(ctx, documentID, chunks[0].IndexVersion, len(chunks), hex.EncodeToString(hash.Sum(nil)), err)
	if err != nil {
		_ = db.DeadLetter(ctx, group, r.Topic, eventID, "SEARCH_PROJECTION_FAILED", 1)
		_ = db.FinishConsumerReceipt(ctx, group, eventID, "failed")
		return
	}
	_ = db.FinishConsumerReceipt(ctx, group, eventID, "success")
}
