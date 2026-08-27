package workers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"cortex/backend/internal/ai"
	"cortex/backend/internal/blobstore"
	"cortex/backend/internal/config"
	"cortex/backend/internal/documentparser"
	"cortex/backend/internal/eventbus"
	"cortex/backend/internal/knowledge"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	parsingConsumerGroup   = "cortex-knowledge-parsing-v1"
	embeddingConsumerGroup = "cortex-knowledge-embedding-v1"
)

func runKnowledgeParsingConsumer(ctx context.Context, cfg config.Config, db *store.Store, blobs, localBlobs blobstore.BlobStore, logger *slog.Logger) {
	parser := documentparser.Client{BaseURL: cfg.DocumentParserURL, Timeout: cfg.DocumentParserTimeout, MaxBody: cfg.KnowledgeMaxFileBytes}
	runStageConsumer(ctx, cfg.KafkaRESTURL, parsingConsumerGroup, "cortex.knowledge.index.v1", logger, func(record eventbus.Record) bool {
		return processKnowledgeParsing(ctx, cfg, db, blobs, localBlobs, parser, record, logger)
	})
}

func processKnowledgeParsing(ctx context.Context, cfg config.Config, db *store.Store, blobs, localBlobs blobstore.BlobStore, parser documentparser.Client, record eventbus.Record, logger *slog.Logger) bool {
	documentID, err := uuid.Parse(record.Value.AggregateID)
	if err != nil {
		return true
	}
	job, err := db.ClaimKnowledgeJobForStage(ctx, documentID, uuid.New(), "parsing", 5*time.Minute)
	if errors.Is(err, pgx.ErrNoRows) {
		return true // duplicate or stale stage event
	}
	if err != nil {
		logger.Error("claim knowledge parsing stage", "code", "KNOWLEDGE_STAGE_CLAIM_FAILED", "error", err)
		return false
	}
	fail := func(code string) bool {
		if failErr := db.FailKnowledgeJob(ctx, job, code); failErr != nil && !errors.Is(failErr, store.ErrKnowledgeIndexLeaseLost) {
			logger.Error("fail knowledge parsing stage", "code", code)
			return false
		}
		return true
	}
	if err = db.LoadKnowledgeJobDocument(ctx, &job); err != nil {
		return fail("KNOWLEDGE_DOCUMENT_UNAVAILABLE")
	}
	content := job.Content
	if job.SourceType == "upload" {
		backend := blobs
		if job.StorageBackend == "local" {
			backend = localBlobs
		}
		reader, _, readErr := backend.Open(ctx, job.StoredPath)
		var data []byte
		if readErr == nil {
			data, readErr = io.ReadAll(io.LimitReader(reader, cfg.KnowledgeMaxFileBytes+1))
			_ = reader.Close()
		}
		if readErr != nil {
			return fail("KNOWLEDGE_FILE_MISSING")
		}
		if int64(len(data)) > cfg.KnowledgeMaxFileBytes {
			return fail("KNOWLEDGE_QUOTA_EXCEEDED")
		}
		if strings.EqualFold(filepath.Ext(job.StoredPath), ".md") {
			content = string(data)
		} else {
			parsed, parseErr := parser.Parse(ctx, filepath.Base(job.StoredPath), data)
			if parseErr != nil {
				code := "KNOWLEDGE_PARSER_UNAVAILABLE"
				var parserErr *documentparser.Error
				if errors.As(parseErr, &parserErr) {
					code = parserErr.Code
				}
				return fail(code)
			}
			content = documentparser.Markdown(parsed)
		}
	}
	parents := knowledge.Chunk(job.Title, job.SourceType, content)
	if len(parents) == 0 {
		return fail("KNOWLEDGE_MARKDOWN_INVALID")
	}
	if err = db.SaveParsedKnowledge(ctx, job, parents); err != nil {
		logger.Error("save parsed knowledge", "document_id", documentID.String(), "code", "KNOWLEDGE_PARSE_PERSIST_FAILED")
		return fail("KNOWLEDGE_INDEX_FAILED")
	}
	return true
}

func runKnowledgeEmbeddingConsumer(ctx context.Context, cfg config.Config, db *store.Store, logger *slog.Logger) {
	client := ai.LocalEmbeddingClient{BaseURL: cfg.EmbeddingBaseURL, APIKey: cfg.EmbeddingAPIKey, Model: cfg.EmbeddingModel, Dimensions: cfg.EmbeddingDimensions, SendDimensions: cfg.EmbeddingSendDimensions, MaxBatchSize: cfg.KnowledgeIndexBatchSize}
	runStageConsumer(ctx, cfg.KafkaRESTURL, embeddingConsumerGroup, "cortex.document.parsed.v1", logger, func(record eventbus.Record) bool {
		return processKnowledgeEmbedding(ctx, cfg, db, client, record, logger)
	})
}

func processKnowledgeEmbedding(ctx context.Context, cfg config.Config, db *store.Store, client ai.LocalEmbeddingClient, record eventbus.Record, logger *slog.Logger) bool {
	documentID, err := uuid.Parse(record.Value.AggregateID)
	if err != nil {
		return true
	}
	job, err := db.ClaimKnowledgeJobForStage(ctx, documentID, uuid.New(), "embedding", 5*time.Minute)
	if errors.Is(err, pgx.ErrNoRows) {
		return true
	}
	if err != nil {
		logger.Error("claim knowledge embedding stage", "code", "KNOWLEDGE_STAGE_CLAIM_FAILED", "error", err)
		return false
	}
	parents, err := db.LoadParsedKnowledge(ctx, job)
	if err != nil {
		return db.FailKnowledgeJob(ctx, job, "KNOWLEDGE_PARSED_ARTIFACT_UNAVAILABLE") == nil
	}
	var texts []string
	for _, parent := range parents {
		for _, child := range parent.Children {
			texts = append(texts, child.EmbeddingText)
		}
	}
	_ = db.UpdateKnowledgeJobProgress(ctx, job, "embedding", 0, len(texts))
	vectors, err := client.Embed(ctx, texts)
	if err != nil {
		logger.Error("knowledge embedding failed", "document_id", documentID.String(), "code", "KNOWLEDGE_EMBEDDING_UNAVAILABLE")
		return db.FailKnowledgeJob(ctx, job, "KNOWLEDGE_EMBEDDING_UNAVAILABLE") == nil
	}
	_ = db.UpdateKnowledgeJobProgress(ctx, job, "persisting", len(texts), len(texts))
	nested := make([][][]float32, len(parents))
	offset := 0
	for pi, parent := range parents {
		nested[pi] = make([][]float32, len(parent.Children))
		copy(nested[pi], vectors[offset:offset+len(parent.Children)])
		offset += len(parent.Children)
	}
	if err = db.WriteKnowledgeChunks(ctx, job, parents, nested, cfg.EmbeddingModel); err != nil {
		logger.Error("knowledge chunk write failed", "document_id", documentID.String(), "code", "KNOWLEDGE_INDEX_FAILED")
		return db.FailKnowledgeJob(ctx, job, "KNOWLEDGE_INDEX_FAILED") == nil
	}
	return true
}

func runStageConsumer(ctx context.Context, kafkaURL, group, topic string, logger *slog.Logger, process func(eventbus.Record) bool) {
	for ctx.Err() == nil {
		consumer, err := eventbus.NewConsumer(ctx, kafkaURL, group, []string{topic})
		if err != nil {
			logger.Error("create kafka stage consumer", "group", group, "code", "KAFKA_CONSUMER_UNAVAILABLE")
			if !wait(ctx, 2*time.Second) {
				return
			}
			continue
		}
		for ctx.Err() == nil {
			records, pollErr := consumer.Poll(ctx)
			if pollErr != nil {
				logger.Error("poll kafka stage", "group", group, "code", "KAFKA_POLL_FAILED")
				break
			}
			processed := true
			for _, record := range records {
				if !process(record) {
					processed = false
					break
				}
			}
			if !processed {
				break // close without committing; Kafka will redeliver
			}
			if err := consumer.Commit(ctx); err != nil {
				logger.Error("commit kafka stage", "group", group, "code", "KAFKA_COMMIT_FAILED")
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
