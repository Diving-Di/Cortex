package searchindex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Document struct {
	ID                                                 string
	TenantID, DocumentID, ParentID                     string
	IndexVersion                                       int
	CollectionID                                       *string
	Title, SourceType, SourcePath, Content, SearchText string
	Heading                                            []string
	Embedding                                          []float32
}

func (e *Elasticsearch) EnsureIndex(ctx context.Context) error {
	mapping := map[string]any{"aliases": map[string]any{e.alias: map[string]any{"is_write_index": true}}, "mappings": map[string]any{"dynamic": "strict", "properties": map[string]any{"tenant_id": map[string]any{"type": "keyword"}, "document_id": map[string]any{"type": "keyword"}, "parent_id": map[string]any{"type": "keyword"}, "index_version": map[string]any{"type": "integer"}, "collection_id": map[string]any{"type": "keyword"}, "title": map[string]any{"type": "text", "analyzer": "standard"}, "heading": map[string]any{"type": "text", "analyzer": "standard"}, "search_text": map[string]any{"type": "text", "analyzer": "standard"}, "content": map[string]any{"type": "text", "index": false}, "source_type": map[string]any{"type": "keyword"}, "source_path": map[string]any{"type": "keyword", "index": false}, "status": map[string]any{"type": "keyword"}, "knowledge_enabled": map[string]any{"type": "boolean"}, "embedding": map[string]any{"type": "dense_vector", "dims": 512, "index": true, "similarity": "cosine"}}}}
	raw, _ := json.Marshal(mapping)
	resp, err := e.do(ctx, http.MethodPut, "/cortex-knowledge-v1-000001", raw)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 && resp.StatusCode != 400 {
		return fmt.Errorf("create index: %d", resp.StatusCode)
	}
	return nil
}
func (e *Elasticsearch) BulkUpsert(ctx context.Context, docs []Document) error {
	var body bytes.Buffer
	enc := json.NewEncoder(&body)
	for _, d := range docs {
		_ = enc.Encode(map[string]any{"index": map[string]any{"_index": e.alias, "_id": d.ID, "routing": d.TenantID}})
		_ = enc.Encode(map[string]any{"tenant_id": d.TenantID, "document_id": d.DocumentID, "parent_id": d.ParentID, "index_version": d.IndexVersion, "collection_id": d.CollectionID, "title": d.Title, "heading": d.Heading, "search_text": d.SearchText, "content": d.Content, "source_type": d.SourceType, "source_path": d.SourcePath, "status": "ready", "knowledge_enabled": true, "embedding": d.Embedding})
	}
	resp, err := e.do(ctx, http.MethodPost, "/_bulk", body.Bytes())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("bulk projection: %d", resp.StatusCode)
	}
	var result struct {
		Errors bool `json:"errors"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if result.Errors {
		return fmt.Errorf("bulk projection contained errors")
	}
	return nil
}

type Candidate struct {
	DocumentID                             uuid.UUID
	ParentID                               uuid.UUID
	IndexVersion                           int
	Title, Content, SourceType, SourcePath string
	Heading                                []string
	Score                                  float64
	RouteProvenance                        int
}
type Elasticsearch struct {
	urls                      []string
	username, password, alias string
	client                    *http.Client
}

func New(urls []string, user, password, alias string) *Elasticsearch {
	return &Elasticsearch{urls: urls, username: user, password: password, alias: alias, client: &http.Client{Timeout: 12 * time.Second}}
}
func (e *Elasticsearch) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	if len(e.urls) == 0 {
		return nil, fmt.Errorf("elasticsearch is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(e.urls[0], "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.username != "" {
		req.SetBasicAuth(e.username, e.password)
	}
	return e.client.Do(req)
}
func (e *Elasticsearch) Ready(ctx context.Context) error {
	resp, err := e.do(ctx, http.MethodGet, "/_cluster/health", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("elasticsearch unavailable")
	}
	return nil
}
func (e *Elasticsearch) HybridSearch(ctx context.Context, tenant uuid.UUID, query string, vector []float32, collections []uuid.UUID, limit int) ([]Candidate, error) {
	filters := []any{map[string]any{"term": map[string]any{"tenant_id": tenant.String()}}, map[string]any{"term": map[string]any{"knowledge_enabled": true}}, map[string]any{"term": map[string]any{"status": "ready"}}}
	if len(collections) > 0 {
		ids := make([]string, len(collections))
		for i, v := range collections {
			ids[i] = v.String()
		}
		filters = append(filters, map[string]any{"terms": map[string]any{"collection_id": ids}})
	}
	body := map[string]any{"size": limit, "query": map[string]any{"bool": map[string]any{"filter": filters, "should": []any{map[string]any{"multi_match": map[string]any{"query": query, "fields": []string{"title^3", "heading^2", "search_text"}}}}, "minimum_should_match": 0}}, "knn": map[string]any{"field": "embedding", "query_vector": vector, "k": limit, "num_candidates": max(limit*5, 100), "filter": map[string]any{"bool": map[string]any{"filter": filters}}}, "_source": []string{"document_id", "parent_id", "index_version", "title", "content", "source_type", "source_path", "heading"}}
	raw, _ := json.Marshal(body)
	resp, err := e.do(ctx, http.MethodPost, "/"+url.PathEscape(e.alias)+"/_search?routing="+url.QueryEscape(tenant.String()), raw)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("elasticsearch search failed with status %d", resp.StatusCode)
	}
	var result struct {
		Hits struct {
			Hits []struct {
				Score  float64 `json:"_score"`
				Source struct {
					DocumentID   string   `json:"document_id"`
					ParentID     string   `json:"parent_id"`
					IndexVersion int      `json:"index_version"`
					Title        string   `json:"title"`
					Content      string   `json:"content"`
					SourceType   string   `json:"source_type"`
					SourcePath   string   `json:"source_path"`
					Heading      []string `json:"heading"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(result.Hits.Hits))
	for _, h := range result.Hits.Hits {
		d, e1 := uuid.Parse(h.Source.DocumentID)
		p, e2 := uuid.Parse(h.Source.ParentID)
		if e1 != nil || e2 != nil {
			continue
		}
		out = append(out, Candidate{DocumentID: d, ParentID: p, IndexVersion: h.Source.IndexVersion, Title: h.Source.Title, Content: h.Source.Content, SourceType: h.Source.SourceType, SourcePath: h.Source.SourcePath, Heading: h.Source.Heading, Score: h.Score, RouteProvenance: 3})
	}
	return out, nil
}
