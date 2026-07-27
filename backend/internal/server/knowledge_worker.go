package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"diary-listener/backend/internal/ai"
	"diary-listener/backend/internal/config"
	"diary-listener/backend/internal/domain"
	"diary-listener/backend/internal/knowledge"
	"diary-listener/backend/internal/store"
	"github.com/google/uuid"
)

func RunKnowledgeIndexer(
	ctx context.Context, cfg config.Config, database *store.Store, logger *slog.Logger,
) {
	owner := "knowledge-" + uuid.NewString()
	embeddingClient := ai.LocalEmbeddingClient{
		BaseURL:        cfg.EmbeddingBaseURL,
		APIKey:         cfg.EmbeddingAPIKey,
		Model:          cfg.EmbeddingModel,
		Dimensions:     cfg.EmbeddingDimensions,
		SendDimensions: cfg.EmbeddingSendDimensions,
		MaxBatchSize:   16,
		MaxRetries:     2,
	}
	workers := max(1, cfg.KnowledgeIndexWorkers)
	for index := 0; index < workers; index++ {
		go runKnowledgeIndexWorker(ctx, cfg, database, logger, embeddingClient, owner)
	}
}

func runKnowledgeIndexWorker(
	ctx context.Context,
	cfg config.Config,
	database *store.Store,
	logger *slog.Logger,
	embeddingClient ai.EmbeddingClient,
	owner string,
) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		jobs, err := database.ClaimKnowledgeIndexJobs(ctx, owner, 1, 5*time.Minute)
		if err != nil && ctx.Err() == nil {
			logger.Error("claim knowledge index job", "error", err)
		}
		for _, job := range jobs {
			processKnowledgeIndexJobSafely(ctx, cfg, database, logger, embeddingClient, job)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func processKnowledgeIndexJobSafely(
	ctx context.Context,
	cfg config.Config,
	database *store.Store,
	logger *slog.Logger,
	embeddingClient ai.EmbeddingClient,
	job store.KnowledgeIndexJob,
) {
	defer func() {
		if recovered := recover(); recovered != nil {
			principal := domain.Principal{
				UserID: job.UserID, TenantID: job.TenantID, TenantActive: true,
			}
			logger.Error("knowledge index job panic", "job_id", job.ID)
			failKnowledgeJob(ctx, database, logger, principal, job, "INDEX_WORKER_PANIC", true)
		}
	}()
	processKnowledgeIndexJob(ctx, cfg, database, logger, embeddingClient, job)
}

func processKnowledgeIndexJob(
	ctx context.Context,
	cfg config.Config,
	database *store.Store,
	logger *slog.Logger,
	embeddingClient ai.EmbeddingClient,
	job store.KnowledgeIndexJob,
) {
	principal := domain.Principal{
		UserID: job.UserID, TenantID: job.TenantID, TenantActive: true,
	}
	documentRow, err := database.GetKnowledgeDocument(ctx, principal, job.DocumentID)
	if err != nil {
		failKnowledgeJob(ctx, database, logger, principal, job, "DOCUMENT_NOT_FOUND", false)
		return
	}
	path, err := safeKnowledgeWorkerPath(cfg.DataDir, documentRow.StoredPath)
	if err != nil {
		failKnowledgeJob(ctx, database, logger, principal, job, "DOCUMENT_PATH_INVALID", false)
		return
	}
	document, err := knowledge.Extract(ctx, path, documentRow.Extension, documentRow.OriginalName, knowledge.ExtractLimits{
		MaxPages: cfg.MaxKnowledgePDFPages, MaxCharacters: cfg.MaxKnowledgeChars, TimeoutSecs: 45,
	})
	if err != nil {
		code := "DOCUMENT_PARSE_FAILED"
		switch {
		case errors.Is(err, knowledge.ErrEncrypted):
			code = "DOCUMENT_ENCRYPTED"
		case errors.Is(err, knowledge.ErrOCRRequired):
			code = "DOCUMENT_OCR_REQUIRED"
		case errors.Is(err, knowledge.ErrParseLimit):
			code = "DOCUMENT_PARSE_LIMIT"
		}
		failKnowledgeJob(ctx, database, logger, principal, job, code, false)
		return
	}
	parents := knowledge.BuildParentChildChunks(document, knowledge.ChunkOptions{
		ParentTargetTokens: cfg.KnowledgeParentTarget,
		ParentMaxTokens:    cfg.KnowledgeParentMax,
		ChildTargetTokens:  cfg.KnowledgeChildTarget,
		ChildMaxTokens:     cfg.KnowledgeChildMax,
		ChildOverlapTokens: cfg.KnowledgeChildOverlap,
	})
	if len(parents) == 0 {
		failKnowledgeJob(ctx, database, logger, principal, job, "DOCUMENT_OCR_REQUIRED", false)
		return
	}
	embeddings := make([][][]float32, len(parents))
	for parentIndex, parent := range parents {
		inputs := make([]string, len(parent.Children))
		for childIndex, child := range parent.Children {
			inputs[childIndex] = child.EmbeddingText
		}
		values, embedErr := embedBatches(ctx, embeddingClient, inputs, 32)
		if embedErr != nil {
			logger.Error("embed knowledge document", "document_id", job.DocumentID, "error", embedErr)
			failKnowledgeJob(ctx, database, logger, principal, job, "EMBEDDING_UNAVAILABLE", true)
			return
		}
		embeddings[parentIndex] = values
	}
	if err := database.CompleteKnowledgeIndex(
		ctx, principal, job, document, parents, embeddings, cfg.EmbeddingModel,
	); err != nil {
		logger.Error("complete knowledge index", "document_id", job.DocumentID, "error", err)
		failKnowledgeJob(ctx, database, logger, principal, job, "DOCUMENT_INDEX_FAILED", true)
	}
}

func embedBatches(
	ctx context.Context, client ai.EmbeddingClient, inputs []string, batchSize int,
) ([][]float32, error) {
	result := make([][]float32, 0, len(inputs))
	for start := 0; start < len(inputs); start += batchSize {
		end := min(len(inputs), start+batchSize)
		values, err := client.Embed(ctx, inputs[start:end])
		if err != nil {
			return nil, err
		}
		result = append(result, values...)
	}
	return result, nil
}

func failKnowledgeJob(
	ctx context.Context,
	database *store.Store,
	logger *slog.Logger,
	principal domain.Principal,
	job store.KnowledgeIndexJob,
	code string,
	retry bool,
) {
	if err := database.FailKnowledgeIndex(ctx, principal, job, code, retry); err != nil && ctx.Err() == nil {
		logger.Error("fail knowledge index job", "job_id", job.ID, "error", err)
	}
}

func safeKnowledgeWorkerPath(dataDir, storedPath string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(storedPath))
	if filepath.IsAbs(cleaned) || cleaned == "." || cleaned == ".." ||
		!strings.HasPrefix(cleaned, "knowledge"+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid knowledge path")
	}
	root := filepath.Join(dataDir, "knowledge")
	target := filepath.Join(dataDir, cleaned)
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid knowledge path")
	}
	return target, nil
}
