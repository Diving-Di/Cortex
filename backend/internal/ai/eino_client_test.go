package ai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type fakeEinoModel struct {
	input []*schema.Message
	err   error
}

func (f *fakeEinoModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return nil, errors.New("unused")
}

func (f *fakeEinoModel) Stream(
	_ context.Context, input []*schema.Message, _ ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return schema.StreamReaderFromArray([]*schema.Message{
		{Role: schema.Assistant, Content: "第一段"},
		{Role: schema.Assistant, Content: "第二段"},
	}), nil
}

func TestEinoClientPreservesAIClientStreamingContract(t *testing.T) {
	model := &fakeEinoModel{}
	client := &EinoClient{Model: model}
	events, err := client.StreamChat(context.Background(), ChatRequest{
		Model: "diary-default",
		Messages: []Message{
			{Role: "system", Content: "规则"},
			{Role: "user", Content: "问题"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var content string
	for event := range events {
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		content += event.Content
	}
	if content != "第一段第二段" {
		t.Fatalf("streamed content = %q", content)
	}
	if len(model.input) != 2 || model.input[0].Role != schema.System || model.input[1].Role != schema.User {
		t.Fatalf("messages were not preserved: %#v", model.input)
	}
}

func TestEinoClientReturnsStableGatewayErrors(t *testing.T) {
	client := &EinoClient{Model: &fakeEinoModel{err: errors.New("upstream 429 secret body")}}
	_, err := client.StreamChat(context.Background(), ChatRequest{Model: "diary-default"})
	if err == nil || !strings.Contains(err.Error(), "AI_RATE_LIMITED") ||
		strings.Contains(err.Error(), "secret") {
		t.Fatalf("error = %v", err)
	}
	if mapped := stableEinoError(context.Background(), errors.New("upstream failed secret body")); strings.Contains(mapped.Error(), "secret") {
		t.Fatal("raw upstream error escaped stable mapping: " + mapped.Error())
	}
}

func TestEinoClientRequiresGatewayKey(t *testing.T) {
	client := &EinoClient{}
	if _, err := client.StreamChat(context.Background(), ChatRequest{}); err == nil ||
		err.Error() != "AI_NOT_CONFIGURED" {
		t.Fatalf("error = %v", err)
	}
}

func TestEinoClientPreservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mapped := stableEinoError(ctx, errors.New("upstream secret response"))
	if !errors.Is(mapped, context.Canceled) {
		t.Fatalf("error = %v", mapped)
	}
	if strings.Contains(mapped.Error(), "secret") {
		t.Fatal("cancelled request leaked upstream response")
	}
}
