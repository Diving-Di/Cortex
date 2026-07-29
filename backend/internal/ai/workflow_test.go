package ai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type workflowModel struct {
	input  []*schema.Message
	chunks []string
	err    error
}

func (m *workflowModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return nil, errors.New("unused")
}

func (m *workflowModel) Stream(
	_ context.Context, input []*schema.Message, _ ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	m.input = input
	if m.err != nil {
		return nil, m.err
	}
	items := make([]*schema.Message, len(m.chunks))
	for index, chunk := range m.chunks {
		items[index] = schema.AssistantMessage(chunk, nil)
	}
	return schema.StreamReaderFromArray(items), nil
}

func collectEvents(events <-chan StreamEvent) (string, error) {
	var result strings.Builder
	for event := range events {
		if event.Err != nil {
			return result.String(), event.Err
		}
		result.WriteString(event.Content)
	}
	return result.String(), nil
}

func TestWorkflowUsesEinoPromptChainForAllOperations(t *testing.T) {
	m := &workflowModel{chunks: []string{`{"title":"标题","summary":"摘要","content":"正文"}`}}
	workflow := Workflow{Client: &EinoClient{Model: m}, Model: "diary-default"}

	events, err := workflow.Organize(context.Background(), "不可信原文", "structured")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = collectEvents(events); err != nil {
		t.Fatal(err)
	}
	if len(m.input) != 2 || m.input[0].Role != schema.System ||
		!strings.Contains(m.input[1].Content, "<untrusted_note>") {
		t.Fatalf("organize prompt boundary = %#v", m.input)
	}

	m.chunks = []string{"报告"}
	events, err = workflow.GenerateReport(context.Background(), "来源材料")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = collectEvents(events)
	if !strings.Contains(m.input[0].Content, "不得虚构") || m.input[1].Content != "来源材料" {
		t.Fatalf("report messages = %#v", m.input)
	}

	m.chunks = []string{"回答"}
	events, err = workflow.AnswerKnowledge(context.Background(), KnowledgeInput{
		Question: "问题", ConversationContext: "历史",
		Evidence: []KnowledgeEvidence{{Citation: "K1", Kind: "文件", Title: "资料", Content: "内容"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = collectEvents(events)
	if !strings.Contains(m.input[0].Content, "不可信数据") ||
		!strings.Contains(m.input[1].Content, "[K1 文件:资料]") {
		t.Fatalf("knowledge system prompt = %q", m.input[0].Content)
	}
}

func TestOrganizeRejectsInvalidStructuredOutputBeforeEmittingContent(t *testing.T) {
	m := &workflowModel{chunks: []string{`{"title":`, `"broken"}`}}
	workflow := Workflow{Client: &EinoClient{Model: m}, Model: "diary-default"}
	events, err := workflow.Organize(context.Background(), "content", "structured")
	if err != nil {
		t.Fatal(err)
	}
	content, streamErr := collectEvents(events)
	if content != "" || streamErr == nil || !strings.Contains(streamErr.Error(), "AI_INVALID_STRUCTURED_OUTPUT") {
		t.Fatalf("content=%q error=%v", content, streamErr)
	}
}

func TestWorkflowMapsTimeoutWithoutLeakingUpstreamError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	m := &workflowModel{err: errors.New("upstream secret response")}
	workflow := Workflow{Client: &EinoClient{Model: m}, Model: "diary-default"}
	_, err := workflow.GenerateReport(ctx, "material")
	if !errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error = %v", err)
	}
}
