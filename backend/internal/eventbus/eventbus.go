package eventbus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Event struct {
	ID            string    `json:"event_id"`
	Type          string    `json:"event_type"`
	AggregateID   string    `json:"aggregate_id"`
	TenantRef     string    `json:"tenant_ref,omitempty"`
	SchemaVersion int       `json:"schema_version"`
	TraceID       string    `json:"trace_id,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
}
type Publisher interface {
	Publish(context.Context, string, string, Event) error
	Ready(context.Context) error
}

// KafkaREST publishes to Kafka through Redpanda's internal HTTP proxy. The
// proxy is not exposed publicly and preserves the same topic/key semantics.
type KafkaREST struct {
	base   string
	client *http.Client
}
type Consumer struct {
	base, group, instance string
	client                *http.Client
	pollMu                sync.Mutex
}
type Record struct {
	Topic     string `json:"topic"`
	Partition int    `json:"partition"`
	Offset    int64  `json:"offset"`
	Value     Event  `json:"value"`
}

func NewConsumer(ctx context.Context, base, group string, topics []string) (*Consumer, error) {
	c := &Consumer{base: strings.TrimRight(base, "/"), group: group, client: &http.Client{Timeout: 35 * time.Second}}
	raw, _ := json.Marshal(map[string]any{"name": "cortex-" + uuid.NewString(), "format": "json", "auto.offset.reset": "earliest", "auto.commit.enable": "false"})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/consumers/"+url.PathEscape(group), bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/vnd.kafka.v2+json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("create kafka consumer: %d", resp.StatusCode)
	}
	var created struct {
		InstanceID string `json:"instance_id"`
		BaseURI    string `json:"base_uri"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, err
	}
	if created.InstanceID == "" {
		return nil, fmt.Errorf("kafka consumer instance missing")
	}
	// The proxy's advertised base_uri may be internal to its container network.
	// Resolve operations through the same configured, trusted endpoint instead.
	c.instance = c.base + "/consumers/" + url.PathEscape(group) + "/instances/" + url.PathEscape(created.InstanceID)
	subscribed := false
	defer func() {
		if !subscribed {
			closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = c.Close(closeCtx)
		}
	}()
	raw, _ = json.Marshal(map[string]any{"topics": topics})
	req, _ = http.NewRequestWithContext(ctx, http.MethodPost, c.instance+"/subscription", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/vnd.kafka.v2+json")
	resp, err = c.client.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("subscribe kafka consumer: %d", resp.StatusCode)
	}
	subscribed = true
	return c, nil
}
func (c *Consumer) Poll(ctx context.Context) ([]Record, error) {
	c.pollMu.Lock()
	defer c.pollMu.Unlock()
	// Keep the server long-poll well below the HTTP client timeout. Concurrent
	// fetches on one proxy consumer can crash older Redpanda fetch sessions.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.instance+"/records?timeout=1000&max_bytes=1048576", nil)
	req.Header.Set("Accept", "application/vnd.kafka.json.v2+json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("poll kafka: %d", resp.StatusCode)
	}
	var records []Record
	err = json.NewDecoder(resp.Body).Decode(&records)
	return records, err
}
func (c *Consumer) Commit(ctx context.Context) error {
	// Kafka REST commits the consumer's current offsets when the request has no
	// explicit offset list. Sending an empty JSON object is rejected by
	// Redpanda because it is not a valid OffsetCommitSeekList.
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.instance+"/offsets", nil)
	req.Header.Set("Content-Type", "application/vnd.kafka.v2+json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("commit kafka: %d", resp.StatusCode)
	}
	return nil
}

func (c *Consumer) Close(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, c.instance, nil)
	req.Header.Set("Content-Type", "application/vnd.kafka.v2+json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("close kafka consumer: %d", resp.StatusCode)
	}
	return nil
}

func NewKafkaREST(base string) *KafkaREST {
	return &KafkaREST{base: strings.TrimRight(base, "/"), client: &http.Client{Timeout: 10 * time.Second}}
}
func (p *KafkaREST) Publish(ctx context.Context, topic, key string, event Event) error {
	body, _ := json.Marshal(map[string]any{"records": []any{map[string]any{"key": key, "value": event}}})
	u := p.base + "/topics/" + url.PathEscape(topic)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/vnd.kafka.json.v2+json")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("kafka publish failed with status %d", resp.StatusCode)
	}
	// HTTP 200 can still contain a per-record broker error. Never mark the
	// transactional outbox published without an acknowledged record offset.
	var result struct {
		Offsets []struct {
			Offset    *int64 `json:"offset"`
			ErrorCode *int   `json:"error_code"`
		} `json:"offsets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&result); err != nil {
		return fmt.Errorf("kafka publish acknowledgement invalid")
	}
	if len(result.Offsets) != 1 || result.Offsets[0].Offset == nil || *result.Offsets[0].Offset < 0 ||
		(result.Offsets[0].ErrorCode != nil && *result.Offsets[0].ErrorCode != 0) {
		return fmt.Errorf("kafka record was not acknowledged")
	}
	return nil
}
func (p *KafkaREST) Ready(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.base+"/brokers", nil)
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("kafka unavailable")
	}
	return nil
}
