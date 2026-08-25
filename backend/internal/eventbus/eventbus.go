package eventbus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
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
}
type Record struct {
	Topic     string `json:"topic"`
	Partition int    `json:"partition"`
	Offset    int64  `json:"offset"`
	Value     Event  `json:"value"`
}

func NewConsumer(ctx context.Context, base, group string, topics []string) (*Consumer, error) {
	c := &Consumer{base: strings.TrimRight(base, "/"), group: group, client: &http.Client{Timeout: 35 * time.Second}}
	raw, _ := json.Marshal(map[string]any{"name": "cortex-" + fmt.Sprint(time.Now().UnixNano()), "format": "json", "auto.offset.reset": "earliest", "auto.commit.enable": "false"})
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
	c.instance = created.BaseURI
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
	return c, nil
}
func (c *Consumer) Poll(ctx context.Context) ([]Record, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.instance+"/records", nil)
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
