package server

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
	"cortex/backend/internal/knowledge"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

func RunKnowledgeIndexer(ctx context.Context, cfg config.Config, s *store.Store, blobs, localBlobs blobstore.BlobStore, logger *slog.Logger) {
	client := ai.LocalEmbeddingClient{BaseURL: cfg.EmbeddingBaseURL, APIKey: cfg.EmbeddingAPIKey, Model: cfg.EmbeddingModel, Dimensions: cfg.EmbeddingDimensions, SendDimensions: cfg.EmbeddingSendDimensions, MaxBatchSize: cfg.KnowledgeIndexBatchSize}
	parser := documentparser.Client{BaseURL: cfg.DocumentParserURL, Timeout: cfg.DocumentParserTimeout, MaxBody: cfg.KnowledgeMaxFileBytes}
	owner := uuid.New()
	go func() {
		ticker := time.NewTicker(time.Duration(cfg.KnowledgeIndexPollSeconds) * time.Second)
		defer ticker.Stop()
		for {
			jobs, err := s.ClaimKnowledgeJobs(ctx, owner, cfg.KnowledgeIndexBatchSize, 5*time.Minute)
			if err != nil {
				logger.Error("knowledge job claim failed", "code", "KNOWLEDGE_INDEX_FAILED")
			} else {
				for _, job := range jobs {
					_ = s.UpdateKnowledgeJobProgress(ctx, job, "loading", 0, 0)
					if err := s.LoadKnowledgeJobDocument(ctx, &job); err != nil {
						if errors.Is(s.FailKnowledgeJob(ctx, job, "KNOWLEDGE_DOCUMENT_UNAVAILABLE"), store.ErrKnowledgeIndexLeaseLost) {
							knowledgeIndexLeaseLost.Add(1)
						}
						continue
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
							reader.Close()
						}
						if readErr != nil {
							if errors.Is(s.FailKnowledgeJob(ctx, job, "KNOWLEDGE_FILE_MISSING"), store.ErrKnowledgeIndexLeaseLost) {
								knowledgeIndexLeaseLost.Add(1)
							}
							continue
						}
						if int64(len(data)) > cfg.KnowledgeMaxFileBytes {
							_ = s.FailKnowledgeJob(ctx, job, "KNOWLEDGE_QUOTA_EXCEEDED")
							continue
						}
						ext := strings.ToLower(filepath.Ext(job.StoredPath))
						if ext == ".md" {
							content = string(data)
						} else {
							parsed, parseErr := parser.Parse(ctx, filepath.Base(job.StoredPath), data)
							if parseErr != nil {
								code := "KNOWLEDGE_PARSER_UNAVAILABLE"
								var parserErr *documentparser.Error
								if errors.As(parseErr, &parserErr) {
									code = parserErr.Code
								}
								logger.Error("knowledge document parse failed", "document_id", job.DocumentID.String(), "code", code)
								_ = s.FailKnowledgeJob(ctx, job, code)
								continue
							}
							content = documentparser.Markdown(parsed)
						}
					}
					_ = s.UpdateKnowledgeJobProgress(ctx, job, "parsing", 0, 0)
					parents := knowledge.Chunk(job.Title, job.SourceType, content)
					if len(parents) == 0 {
						if errors.Is(s.FailKnowledgeJob(ctx, job, "KNOWLEDGE_MARKDOWN_INVALID"), store.ErrKnowledgeIndexLeaseLost) {
							knowledgeIndexLeaseLost.Add(1)
						}
						continue
					}
					var texts []string
					for _, p := range parents {
						for _, child := range p.Children {
							texts = append(texts, child.EmbeddingText)
						}
					}
					_ = s.UpdateKnowledgeJobProgress(ctx, job, "embedding", 0, len(texts))
					vectors, embedErr := client.Embed(ctx, texts)
					if embedErr != nil {
						logger.Error("knowledge embedding failed", "document_id", job.DocumentID.String(), "code", "KNOWLEDGE_EMBEDDING_UNAVAILABLE", "error", embedErr)
						if errors.Is(s.FailKnowledgeJob(ctx, job, "KNOWLEDGE_EMBEDDING_UNAVAILABLE"), store.ErrKnowledgeIndexLeaseLost) {
							knowledgeIndexLeaseLost.Add(1)
						}
						continue
					}
					_ = s.UpdateKnowledgeJobProgress(ctx, job, "persisting", len(texts), len(texts))
					nested := make([][][]float32, len(parents))
					offset := 0
					for pi, p := range parents {
						nested[pi] = make([][]float32, len(p.Children))
						copy(nested[pi], vectors[offset:offset+len(p.Children)])
						offset += len(p.Children)
					}
					if err := s.WriteKnowledgeChunks(ctx, job, parents, nested, cfg.EmbeddingModel); err != nil {
						logger.Error("knowledge chunk write failed", "document_id", job.DocumentID.String(), "code", "KNOWLEDGE_INDEX_FAILED", "error", err)
						if errors.Is(s.FailKnowledgeJob(ctx, job, "KNOWLEDGE_INDEX_FAILED"), store.ErrKnowledgeIndexLeaseLost) {
							knowledgeIndexLeaseLost.Add(1)
						}
					}
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
