package recipe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"diary-listener/backend/internal/store"
)

// Retriever wraps search and rerank logic for recipes.
type Retriever struct {
	Store          *store.Store
	RerankURL      string
	RerankModel    string
	EmbeddingURL   string
	EmbeddingModel string
	HTTPClient     *http.Client
}

const EmbeddingDimensions = 512

// Search returns semantic candidates. Recipe requests fail closed when the
// required embedding service is unavailable; they never mix legacy vectors.
func (r *Retriever) Search(ctx context.Context, query string, limit int) ([]store.RecipeCandidate, error) {
	if r == nil || r.Store == nil || r.EmbeddingURL == "" || r.EmbeddingModel == "" {
		return nil, errors.New("recipe embedding is not configured")
	}
	client := r.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 6 * time.Second}
	}
	reqBody := map[string]any{"input": query, "model": r.EmbeddingModel}
	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", r.EmbeddingURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("recipe embedding unavailable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("recipe embedding returned status %d", resp.StatusCode)
	}
	var parsed map[string]any
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
					if len(vec) != EmbeddingDimensions {
						return nil, fmt.Errorf("recipe embedding dimensions: got %d want %d", len(vec), EmbeddingDimensions)
					}
					return r.Store.SearchRecipesByVector(ctx, vec, r.EmbeddingModel, limit)
				}
			}
		}
	}
	return nil, errors.New("recipe embedding response is invalid")
}

type rerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
}

type rerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

type rerankResponse struct {
	Results []rerankResult `json:"results"`
}

// Rerank orders candidates using an external reranker service. On error, returns original order.
func (r *Retriever) Rerank(ctx context.Context, query string, candidates []store.RecipeCandidate) ([]store.RecipeCandidate, error) {
	if len(candidates) == 0 {
		return candidates, nil
	}
	if r == nil || r.RerankURL == "" || r.RerankModel == "" {
		return nil, errors.New("recipe reranker is not configured")
	}
	docs := make([]string, 0, len(candidates))
	for _, c := range candidates {
		docs = append(docs, c.Snippet)
	}
	reqBody := rerankRequest{Model: r.RerankModel, Query: query, Documents: docs}
	b, _ := json.Marshal(reqBody)
	client := r.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, "POST", r.RerankURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("recipe reranker unavailable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("recipe reranker returned status %d", resp.StatusCode)
	}
	var rr rerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return nil, errors.New("recipe reranker response is invalid")
	}
	// build new ordering
	ordered := make([]store.RecipeCandidate, 0, len(candidates))
	seen := make([]bool, len(candidates))
	for _, res := range rr.Results {
		if res.Index >= 0 && res.Index < len(candidates) && !seen[res.Index] {
			ordered = append(ordered, candidates[res.Index])
			seen[res.Index] = true
		}
	}
	// append any missing
	for i, c := range candidates {
		if !seen[i] {
			ordered = append(ordered, c)
		}
	}
	return ordered, nil
}
