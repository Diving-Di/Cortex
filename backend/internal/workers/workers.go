package workers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"cortex/backend/internal/blobstore"
	"cortex/backend/internal/config"
	"cortex/backend/internal/eventbus"
	"cortex/backend/internal/searchindex"
	"cortex/backend/internal/server"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

const projectionConsumerGroup = "cortex-search-projection-v1"

// Run starts the infrastructure workers owned by the server process. Every
// worker shares the server cancellation context so deployments have one Go
// entrypoint and one graceful-shutdown boundary.
func Run(ctx context.Context, cfg config.Config, db *store.Store, blobs, localBlobs, minioBlobs blobstore.BlobStore, logger *slog.Logger) {
	go runObjectGC(ctx, db, localBlobs, minioBlobs, logger)
	if cfg.EventBus != "kafka" {
		server.RunKnowledgeIndexer(ctx, cfg, db, blobs, localBlobs, logger)
		return
	}

	go runOutboxRelay(ctx, cfg, db, logger)
	go runKnowledgeParsingConsumer(ctx, cfg, db, blobs, localBlobs, logger)
	go runKnowledgeEmbeddingConsumer(ctx, cfg, db, logger)
	if cfg.RAGRetrievalBackend == "elasticsearch" {
		go runProjectionConsumer(ctx, cfg, db, logger)
	}
}

func runOutboxRelay(ctx context.Context, cfg config.Config, db *store.Store, logger *slog.Logger) {
	publisher := eventbus.NewKafkaREST(cfg.KafkaRESTURL)
	owner := "relay-" + uuid.NewString()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		// Template events are consumed directly by the marketplace projector.
		// This relay owns only knowledge pipeline events, preventing competing
		// consumers from claiming the same PostgreSQL outbox row.
		event, err := db.ClaimOutboxEvent(ctx, "knowledge", owner, 30*time.Second)
		if err == nil && event != nil {
			message := eventbus.Event{ID: event.ID, Type: event.EventType, AggregateID: event.AggregateID, SchemaVersion: 1, OccurredAt: event.OccurredAt}
			err = publisher.Publish(ctx, topicFor(event.EventType), event.AggregateID, message)
			_ = db.FinishOutboxEvent(ctx, event.ID, owner, err)
			continue
		}
		if err != nil && ctx.Err() == nil {
			logger.Error("outbox claim failed", "code", "OUTBOX_CLAIM_FAILED")
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
	case eventType == "knowledge.document.parsed":
		return "cortex.document.parsed.v1"
	case strings.HasPrefix(eventType, "knowledge."):
		return "cortex.knowledge.index.v1"
	case strings.HasPrefix(eventType, "search."):
		return "cortex.search.projection.v1"
	default:
		return "cortex.audit.export.v1"
	}
}

func runObjectGC(ctx context.Context, db *store.Store, localBlobs, minioBlobs blobstore.BlobStore, logger *slog.Logger) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		job, err := db.ClaimObjectGC(ctx, 2*time.Minute)
		if err != nil {
			if ctx.Err() == nil {
				logger.Error("claim object gc", "code", "OBJECT_GC_CLAIM_FAILED")
			}
		} else if job != nil {
			backend, resolveErr := objectGCBackend(job.Backend, localBlobs, minioBlobs)
			deleteErr := resolveErr
			if deleteErr == nil {
				deleteErr = backend.Delete(ctx, job.Key, job.ObjectVersion)
			}
			_ = db.FinishObjectGC(ctx, *job, deleteErr == nil)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func objectGCBackend(name string, localBlobs, minioBlobs blobstore.BlobStore) (blobstore.BlobStore, error) {
	switch name {
	case "local":
		if localBlobs != nil {
			return localBlobs, nil
		}
	case "minio":
		if minioBlobs != nil {
			return minioBlobs, nil
		}
	default:
		return nil, fmt.Errorf("unsupported object storage backend")
	}
	return nil, fmt.Errorf("object storage backend is not configured")
}

func runProjectionConsumer(ctx context.Context, cfg config.Config, db *store.Store, logger *slog.Logger) {
	es := searchindex.New(cfg.ElasticsearchURLs, cfg.ElasticsearchUsername, cfg.ElasticsearchPassword, cfg.ElasticsearchIndexAlias)
	for ctx.Err() == nil {
		if err := es.EnsureIndex(ctx); err == nil {
			break
		}
		logger.Error("ensure search index", "code", "SEARCH_INDEX_UNAVAILABLE")
		if !wait(ctx, 2*time.Second) {
			return
		}
	}
	for ctx.Err() == nil {
		consumer, err := eventbus.NewConsumer(ctx, cfg.KafkaRESTURL, projectionConsumerGroup, []string{"cortex.search.projection.v1"})
		if err != nil {
			logger.Error("create kafka consumer", "code", "KAFKA_CONSUMER_UNAVAILABLE")
			if !wait(ctx, 2*time.Second) {
				return
			}
			continue
		}
		for ctx.Err() == nil {
			records, pollErr := consumer.Poll(ctx)
			if pollErr != nil {
				logger.Error("poll kafka", "code", "KAFKA_POLL_FAILED")
				break
			}
			processed := true
			for _, record := range records {
				if !processProjection(ctx, db, es, record) {
					processed = false
					break
				}
			}
			if !processed {
				break
			}
			if err := consumer.Commit(ctx); err != nil {
				logger.Error("commit kafka", "code", "KAFKA_COMMIT_FAILED")
				break
			}
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = consumer.Close(closeCtx)
		cancel()
		if !wait(ctx, time.Second) {
			return
		}
	}
}

func processProjection(ctx context.Context, db *store.Store, es *searchindex.Elasticsearch, record eventbus.Record) bool {
	eventID, eventErr := uuid.Parse(record.Value.ID)
	documentID, documentErr := uuid.Parse(record.Value.AggregateID)
	if eventErr != nil || documentErr != nil {
		return true
	}
	done, err := db.ConsumerSucceeded(ctx, projectionConsumerGroup, eventID)
	if err != nil {
		return false
	}
	if done {
		return true
	}
	chunks, err := db.LoadSearchProjection(ctx, documentID)
	if err != nil || len(chunks) == 0 {
		_ = db.DeadLetter(ctx, projectionConsumerGroup, record.Topic, eventID, "PROJECTION_SOURCE_UNAVAILABLE", 1)
		return false
	}
	docs := make([]searchindex.Document, len(chunks))
	hash := sha256.New()
	for i, chunk := range chunks {
		var collection *string
		if chunk.CollectionID != nil {
			value := chunk.CollectionID.String()
			collection = &value
		}
		docs[i] = searchindex.Document{ID: chunk.ChunkID.String(), TenantID: chunk.TenantID.String(), DocumentID: chunk.DocumentID.String(), ParentID: chunk.ParentID.String(), IndexVersion: chunk.IndexVersion, CollectionID: collection, Title: chunk.Title, SourceType: chunk.SourceType, SourcePath: chunk.SourcePath, Content: chunk.Content, SearchText: chunk.SearchText, Heading: chunk.Heading, Embedding: chunk.Embedding}
		_, _ = hash.Write([]byte(docs[i].ID))
	}
	err = es.BulkUpsert(ctx, docs)
	_ = db.CompleteSearchProjection(ctx, documentID, chunks[0].IndexVersion, len(chunks), hex.EncodeToString(hash.Sum(nil)), err)
	if err != nil {
		_ = db.DeadLetter(ctx, projectionConsumerGroup, record.Topic, eventID, "SEARCH_PROJECTION_FAILED", 1)
		return false
	}
	if _, err = db.ConsumerReceived(ctx, projectionConsumerGroup, eventID); err != nil {
		return false
	}
	_ = db.FinishConsumerReceipt(ctx, projectionConsumerGroup, eventID, "success")
	return true
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
