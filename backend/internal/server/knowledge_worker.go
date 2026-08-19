package server

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"cortex/backend/internal/ai"
	"cortex/backend/internal/config"
	"cortex/backend/internal/knowledge"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

func RunKnowledgeIndexer(ctx context.Context, cfg config.Config, s *store.Store, logger *slog.Logger) {
	client := ai.LocalEmbeddingClient{BaseURL: cfg.EmbeddingBaseURL, APIKey: cfg.EmbeddingAPIKey, Model: cfg.EmbeddingModel, Dimensions: cfg.EmbeddingDimensions, SendDimensions: cfg.EmbeddingSendDimensions, MaxBatchSize: cfg.KnowledgeIndexBatchSize}
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
						abs, pathErr := safeDataPath(cfg.DataDir, job.StoredPath)
						if pathErr != nil {
							if errors.Is(s.FailKnowledgeJob(ctx, job, "KNOWLEDGE_ARCHIVE_UNSAFE"), store.ErrKnowledgeIndexLeaseLost) {
								knowledgeIndexLeaseLost.Add(1)
							}
							continue
						}
						data, readErr := os.ReadFile(abs)
						if readErr != nil {
							if errors.Is(s.FailKnowledgeJob(ctx, job, "KNOWLEDGE_FILE_MISSING"), store.ErrKnowledgeIndexLeaseLost) {
								knowledgeIndexLeaseLost.Add(1)
							}
							continue
						}
						content = string(data)
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
