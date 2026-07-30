package recipe

import (
	"bytes"
	"context"
	"encoding/json"
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
						r, err := tx.Query(ctx, `SELECT id,document_id,content_hash,embedding_text FROM recipe_child_chunks WHERE document_id=$1 AND (embedding IS NULL OR embedding_model IS NULL)`, job.DocumentID)
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
						_ = s.CompleteRecipeIndex(ctx, job)
						continue
					}
					success := true
					for _, r := range rows {
						slog.Info("indexer: embedding candidate", "document_id", r.DocumentID, "content_hash", r.ContentHash)
						reqBody := map[string]any{"input": r.EmbeddingText, "model": embeddingModel}
						b, _ := json.Marshal(reqBody)
						req, err := http.NewRequestWithContext(ctx, "POST", embeddingURL, bytes.NewReader(b))
						if err != nil {
							slog.Error("indexer: build request failed", "code", "EMBEDDING_REQUEST_INVALID")
							success = false
							continue
						}
						req.Header.Set("Content-Type", "application/json")
						resp, err := client.Do(req)
						if err != nil {
							slog.Error("indexer: embedding call failed", "code", "EMBEDDING_UNAVAILABLE")
							success = false
							continue
						}
						if resp.StatusCode != http.StatusOK {
							slog.Error("indexer: embedding service returned non-200", "status", resp.StatusCode)
							resp.Body.Close()
							success = false
							continue
						}
						var parsed map[string]any
						wroteEmbedding := false
						if err := json.NewDecoder(resp.Body).Decode(&parsed); err == nil {
							if data, ok := parsed["data"].([]any); ok && len(data) > 0 {
								if first, ok := data[0].(map[string]any); ok {
									if emb, ok := first["embedding"].([]any); ok {
										vec := make([]float32, 0, len(emb))
										for _, v := range emb {
											if f, ok := v.(float64); ok {
												vec = append(vec, float32(f))
											}
										}
										if len(vec) == EmbeddingDimensions {
											if err := s.UpdateRecipeChildEmbedding(ctx, r.DocumentID, r.ContentHash, vec, embeddingModel); err != nil {
												slog.Error("indexer: write embedding failed", "err", err)
												success = false
											} else {
												wroteEmbedding = true
											}
										}
									}
								}
							}
						}
						resp.Body.Close()
						if !wroteEmbedding {
							slog.Error("indexer: invalid embedding response", "code", "EMBEDDING_INVALID_RESPONSE")
							success = false
						}
					}
					if success {
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
