package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func FormatRerankDocument(title, sourceType string, heading []string, content string) string {
	return fmt.Sprintf("标题：%s\n来源：%s\n章节：%s\n内容：%s",
		strings.TrimSpace(title), strings.TrimSpace(sourceType), strings.Join(heading, " / "), strings.TrimSpace(content))
}

type EmbeddingClient interface {
	Embed(ctx context.Context, inputs []string) ([][]float32, error)
}

type RerankClient interface {
	Rerank(ctx context.Context, query string, documents []string) ([]float64, error)
}

type LocalEmbeddingClient struct {
	BaseURL        string
	APIKey         string
	Model          string
	Dimensions     int
	SendDimensions bool
	MaxBatchSize   int
	MaxRetries     int
	RetryBaseDelay time.Duration
	HTTPClient     *http.Client
}

func (c LocalEmbeddingClient) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	batchSize := c.MaxBatchSize
	if batchSize <= 0 {
		batchSize = 16
	}
	result := make([][]float32, 0, len(inputs))
	for start := 0; start < len(inputs); start += batchSize {
		end := min(len(inputs), start+batchSize)
		batch, err := c.embedBatch(ctx, inputs[start:end])
		if err != nil {
			return nil, fmt.Errorf("embedding batch %d: %w", start/batchSize, err)
		}
		result = append(result, batch...)
	}
	return result, nil
}

func (c LocalEmbeddingClient) embedBatch(ctx context.Context, inputs []string) ([][]float32, error) {
	requestBody := map[string]any{
		"model": c.Model, "input": inputs, "encoding_format": "float",
	}
	if c.SendDimensions {
		requestBody["dimensions"] = c.embeddingDimensions()
	}
	payload, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(c.BaseURL, "/") + "/embeddings"
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	response, err := doModelRequest(ctx, client, endpoint, payload, c.APIKey, c.MaxRetries, c.RetryBaseDelay)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("embedding service returned status %d", response.StatusCode)
	}
	var decoded struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<20)).Decode(&decoded); err != nil {
		return nil, err
	}
	if len(decoded.Data) != len(inputs) {
		return nil, fmt.Errorf("embedding count mismatch: got %d want %d", len(decoded.Data), len(inputs))
	}
	result := make([][]float32, len(inputs))
	for _, item := range decoded.Data {
		if item.Index < 0 || item.Index >= len(result) ||
			len(item.Embedding) != c.embeddingDimensions() {
			return nil, fmt.Errorf("invalid embedding response")
		}
		result[item.Index] = item.Embedding
	}
	for _, item := range result {
		if len(item) != c.embeddingDimensions() {
			return nil, fmt.Errorf("missing embedding")
		}
	}
	return result, nil
}

func (c LocalEmbeddingClient) embeddingDimensions() int {
	if c.Dimensions > 0 {
		return c.Dimensions
	}
	return 1024
}

type LocalRerankClient struct {
	BaseURL        string
	Model          string
	MaxDocuments   int
	MaxRetries     int
	RetryBaseDelay time.Duration
	HTTPClient     *http.Client
}

func (c LocalRerankClient) Rerank(ctx context.Context, query string, documents []string) ([]float64, error) {
	if len(documents) == 0 {
		return nil, nil
	}
	maxDocuments := c.MaxDocuments
	if maxDocuments <= 0 {
		maxDocuments = 20
	}
	if len(documents) > maxDocuments {
		return nil, fmt.Errorf("rerank document count %d exceeds limit %d", len(documents), maxDocuments)
	}
	payload, err := json.Marshal(map[string]any{
		"model": c.Model, "query": query, "documents": documents,
	})
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(c.BaseURL, "/") + "/rerank"
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	response, err := doModelRequest(ctx, client, endpoint, payload, "", c.MaxRetries, c.RetryBaseDelay)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("rerank service returned status %d", response.StatusCode)
	}
	var decoded struct {
		Results []struct {
			Index int     `json:"index"`
			Score float64 `json:"relevance_score"`
		} `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&decoded); err != nil {
		return nil, err
	}
	if len(decoded.Results) != len(documents) {
		return nil, fmt.Errorf("rerank count mismatch: got %d want %d", len(decoded.Results), len(documents))
	}
	result := make([]float64, len(documents))
	seen := make([]bool, len(documents))
	for _, item := range decoded.Results {
		if item.Index < 0 || item.Index >= len(result) || seen[item.Index] {
			return nil, fmt.Errorf("invalid rerank response")
		}
		result[item.Index] = item.Score
		seen[item.Index] = true
	}
	return result, nil
}

func doModelRequest(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	payload []byte,
	apiKey string,
	maxRetries int,
	baseDelay time.Duration,
) (*http.Response, error) {
	if maxRetries < 0 {
		maxRetries = 0
	}
	if maxRetries > 3 {
		maxRetries = 3
	}
	if baseDelay <= 0 {
		baseDelay = 100 * time.Millisecond
	}
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		request.Header.Set("Content-Type", "application/json")
		if strings.TrimSpace(apiKey) != "" {
			request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
		}
		response, err := client.Do(request)
		if err == nil && !retryableModelStatus(response.StatusCode) {
			return response, nil
		}
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			_ = response.Body.Close()
			lastErr = fmt.Errorf("model service returned retryable status %d", response.StatusCode)
		} else {
			lastErr = err
		}
		if attempt == maxRetries {
			break
		}
		timer := time.NewTimer(baseDelay * time.Duration(1<<attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

func retryableModelStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}
