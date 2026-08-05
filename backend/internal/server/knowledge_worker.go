package server

import (
	"context"
	"log/slog"
	"os"
	"time"

	"diary-listener/backend/internal/ai"
	"diary-listener/backend/internal/config"
	"diary-listener/backend/internal/knowledge"
	"diary-listener/backend/internal/store"
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
					if err := s.LoadKnowledgeJobDocument(ctx, &job); err != nil {
						_ = s.FailKnowledgeJob(ctx, job, "KNOWLEDGE_DOCUMENT_UNAVAILABLE")
						continue
					}
					content := job.Content
					if job.SourceType == "upload" {
						abs, pathErr := safeDataPath(cfg.DataDir, job.StoredPath)
						if pathErr != nil {
							_ = s.FailKnowledgeJob(ctx, job, "KNOWLEDGE_ARCHIVE_UNSAFE")
							continue
						}
						data, readErr := os.ReadFile(abs)
						if readErr != nil {
							_ = s.FailKnowledgeJob(ctx, job, "KNOWLEDGE_FILE_MISSING")
							continue
						}
						content = string(data)
					}
					parents := knowledge.Chunk(job.Title, job.SourceType, content)
					if len(parents) == 0 {
						_ = s.FailKnowledgeJob(ctx, job, "KNOWLEDGE_MARKDOWN_INVALID")
						continue
					}
					var texts []string
					for _, p := range parents {
						for _, child := range p.Children {
							texts = append(texts, child.EmbeddingText)
						}
					}
					vectors, embedErr := client.Embed(ctx, texts)
					if embedErr != nil {
						logger.Error("knowledge embedding failed", "document_id", job.DocumentID.String(), "code", "KNOWLEDGE_EMBEDDING_UNAVAILABLE", "error", embedErr)
						_ = s.FailKnowledgeJob(ctx, job, "KNOWLEDGE_EMBEDDING_UNAVAILABLE")
						continue
					}
					nested := make([][][]float32, len(parents))
					offset := 0
					for pi, p := range parents {
						nested[pi] = make([][]float32, len(p.Children))
						copy(nested[pi], vectors[offset:offset+len(p.Children)])
						offset += len(p.Children)
					}
					if err := s.WriteKnowledgeChunks(ctx, job, parents, nested, cfg.EmbeddingModel); err != nil {
						logger.Error("knowledge chunk write failed", "document_id", job.DocumentID.String(), "code", "KNOWLEDGE_INDEX_FAILED", "error", err)
						_ = s.FailKnowledgeJob(ctx, job, "KNOWLEDGE_INDEX_FAILED")
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
