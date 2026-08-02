package recipe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
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
	VectorTopK     int
	TitleTopK      int
	KeywordTopK    int
	FusionTopK     int
	ContextTopK    int
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
	vectorTopK := r.VectorTopK
	if vectorTopK <= 0 {
		vectorTopK = max(15, limit)
	}
	titleTopK := r.TitleTopK
	if titleTopK <= 0 {
		titleTopK = 10
	}
	keywordTopK := r.KeywordTopK
	if keywordTopK <= 0 {
		keywordTopK = 15
	}
	fusionTopK := r.FusionTopK
	if fusionTopK <= 0 {
		fusionTopK = max(20, limit)
	}
	queries := ExpandRetrievalQueries(query)
	routes := make([][]store.RecipeCandidate, 0, len(queries)+2)
	for _, variant := range queries {
		vec, err := r.embedQuery(ctx, client, variant.Text)
		if err != nil {
			return nil, err
		}
		items, err := r.Store.SearchRecipesByVector(ctx, vec, r.EmbeddingModel, vectorTopK)
		if err != nil {
			return nil, err
		}
		for i := range items {
			items[i].Routes = []string{"vector_" + variant.Kind}
		}
		routes = append(routes, items)
	}
	intent := queries[len(queries)-1].Kind
	tokens := retrievalTokens(query)
	if items, err := r.Store.SearchRecipesByTitle(ctx, query, intent, titleTopK); err == nil {
		for i := range items {
			items[i].Routes = []string{"title"}
		}
		routes = append(routes, items)
	}
	if items, err := r.Store.SearchRecipesByKeywords(ctx, query, tokens, intent, keywordTopK); err == nil {
		for i := range items {
			items[i].Routes = []string{"keyword"}
		}
		routes = append(routes, items)
	}
	return fuseCandidates(routes, 60, fusionTopK), nil
}

func (r *Retriever) embedQuery(ctx context.Context, client *http.Client, query string) ([]float32, error) {
	b, _ := json.Marshal(map[string]any{"input": query, "model": r.EmbeddingModel})
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
	var parsed struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil || len(parsed.Data) == 0 {
		return nil, errors.New("recipe embedding response is invalid")
	}
	vec := parsed.Data[0].Embedding
	if len(vec) != EmbeddingDimensions {
		return nil, fmt.Errorf("recipe embedding dimensions: got %d want %d", len(vec), EmbeddingDimensions)
	}
	return vec, nil
}

func retrievalTokens(query string) []string {
	fields := strings.FieldsFunc(query, func(r rune) bool { return strings.ContainsRune("，。！？、；：,.!?;:（）() ", r) })
	seen := map[string]bool{}
	var out []string
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if len([]rune(f)) < 2 || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

func fuseCandidates(routes [][]store.RecipeCandidate, k float64, limit int) []store.RecipeCandidate {
	byID := map[int64]*store.RecipeCandidate{}
	for _, route := range routes {
		seen := map[int64]bool{}
		for rank, item := range route {
			if seen[item.ChunkID] {
				continue
			}
			seen[item.ChunkID] = true
			existing := byID[item.ChunkID]
			if existing == nil {
				copy := item
				existing = &copy
				byID[item.ChunkID] = existing
			}
			existing.FusionScore += 1 / (k + float64(rank+1))
			existing.Routes = appendUnique(existing.Routes, item.Routes...)
		}
	}
	result := make([]store.RecipeCandidate, 0, len(byID))
	for _, v := range byID {
		result = append(result, *v)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].FusionScore == result[j].FusionScore {
			return result[i].ChunkID < result[j].ChunkID
		}
		return result[i].FusionScore > result[j].FusionScore
	})
	return result[:min(limit, len(result))]
}
func appendUnique(values []string, more ...string) []string {
	seen := map[string]bool{}
	for _, v := range values {
		seen[v] = true
	}
	for _, v := range more {
		if !seen[v] {
			values = append(values, v)
			seen[v] = true
		}
	}
	return values
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
	if len(rr.Results) != len(candidates) {
		return nil, errors.New("recipe reranker response is incomplete")
	}
	// build new ordering
	ordered := make([]store.RecipeCandidate, 0, len(candidates))
	seen := make([]bool, len(candidates))
	for _, res := range rr.Results {
		if res.Index >= 0 && res.Index < len(candidates) && !seen[res.Index] {
			candidate := candidates[res.Index]
			score := res.RelevanceScore
			candidate.RerankScore = &score
			ordered = append(ordered, candidate)
			seen[res.Index] = true
		}
	}
	for _, present := range seen {
		if !present {
			return nil, errors.New("recipe reranker response has duplicate or invalid indexes")
		}
	}
	contextTopK := r.ContextTopK
	if contextTopK <= 0 {
		contextTopK = 5
	}
	if r.Store != nil {
		return r.Store.ExpandRecipeParents(ctx, ordered, contextTopK, 2)
	}
	return ordered, nil
}
