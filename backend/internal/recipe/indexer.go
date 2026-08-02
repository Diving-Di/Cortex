package recipe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"diary-listener/backend/internal/store"
	"github.com/jackc/pgx/v5"
)

type childRow struct {
	ID            int64  `json:"id"`
	DocumentID    int64  `json:"document_id"`
	ContentHash   string `json:"content_hash"`
	EmbeddingText string `json:"embedding_text"`
}

// StartRecipeIndexer runs background workers that find un-embedded child chunks
// and call the embedding service to write embeddings back into the DB.
func StartRecipeIndexer(ctx context.Context, s *store.Store, embeddingURL, embeddingModel string, workers, batchSize int, pollInterval time.Duration) {
	if embeddingURL == "" || embeddingModel == "" || workers <= 0 {
		return
	}
	slog.Info("recipe indexer starting", "workers", workers, "batchSize", batchSize, "pollInterval", pollInterval.String())
	client := &http.Client{Timeout: 10 * time.Second}
	for w := 0; w < workers; w++ {
		go func(id int) {
			ticker := time.NewTicker(pollInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				// claim recipe index jobs and process per-document
				owner := "recipe-indexer"
				lease := 5 * time.Minute
				jobs, err := s.ClaimRecipeIndexJobs(ctx, owner, batchSize, lease)
				if err != nil {
					slog.Error("indexer: claim jobs failed", "err", err)
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						continue
					}
				}
				if len(jobs) == 0 {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						continue
					}
				}
				for _, job := range jobs {
					var rows []childRow
					_ = s.WithTx(ctx, func(tx pgx.Tx) error {
						r, err := tx.Query(ctx, `SELECT id,document_id,content_hash,embedding_text FROM recipe_child_chunks WHERE document_id=$1 AND index_version=$2 AND (embedding IS NULL OR embedding_model IS NULL)`, job.DocumentID, job.TargetIndexVersion)
						if err != nil {
							return err
						}
						defer r.Close()
						for r.Next() {
							var cr childRow
							if err := r.Scan(&cr.ID, &cr.DocumentID, &cr.ContentHash, &cr.EmbeddingText); err != nil {
								return err
							}
							rows = append(rows, cr)
						}
						return r.Err()
					})
					if len(rows) == 0 {
						if err := s.ActivateRecipeIndex(ctx, job.DocumentID, job.TargetIndexVersion, embeddingModel); err != nil {
							_ = s.FailRecipeIndex(ctx, job, "INDEX_ACTIVATION_FAILED", true)
							continue
						}
						_ = s.CompleteRecipeIndex(ctx, job)
						continue
					}
					texts := make([]string, len(rows))
					for i := range rows {
						texts[i] = rows[i].EmbeddingText
					}
					vectors := make([][]float32, len(rows))
					success := true
					for i := range rows {
						one, err := embedRecipeInput(ctx, client, embeddingURL, embeddingModel, texts[i], 1)
						if err != nil {
							slog.Error("indexer: embedding failed", "document_id", rows[i].DocumentID, "code", "EMBEDDING_UNAVAILABLE")
							success = false
							continue
						}
						vectors[i] = one[0]
					}
					for i, row := range rows {
						if vectors[i] == nil {
							slog.Error("indexer: write embedding failed", "document_id", row.DocumentID, "code", "EMBEDDING_MISSING")
							success = false
							continue
						}
						if err := s.UpdateRecipeChildEmbeddingByID(ctx, row.ID, vectors[i], embeddingModel); err != nil {
							slog.Error("indexer: write embedding failed", "document_id", row.DocumentID, "err", err)
							success = false
						}
					}
					if success {
						if err := s.ActivateRecipeIndex(ctx, job.DocumentID, job.TargetIndexVersion, embeddingModel); err != nil {
							_ = s.FailRecipeIndex(ctx, job, "INDEX_ACTIVATION_FAILED", true)
							continue
						}
						_ = s.CompleteRecipeIndex(ctx, job)
					} else {
						_ = s.FailRecipeIndex(ctx, job, "EMBEDDING_FAILED", true)
					}
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}(w)
	}
}

func embedRecipeTexts(ctx context.Context, client *http.Client, url, model string, texts []string) ([][]float32, error) {
	return embedRecipeInput(ctx, client, url, model, texts, len(texts))
}

func embedRecipeInput(ctx context.Context, client *http.Client, url, model string, input any, expected int) ([][]float32, error) {
	body, _ := json.Marshal(map[string]any{"input": input, "model": model})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding status %d", resp.StatusCode)
	}
	var parsed struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil || len(parsed.Data) != expected {
		return nil, errors.New("invalid embedding batch response")
	}
	vectors := make([][]float32, len(parsed.Data))
	for i := range parsed.Data {
		if len(parsed.Data[i].Embedding) != EmbeddingDimensions {
			return nil, errors.New("invalid embedding dimensions")
		}
		vectors[i] = parsed.Data[i].Embedding
	}
	return vectors, nil
}
