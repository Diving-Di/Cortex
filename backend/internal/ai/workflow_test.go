package ai

import (
	"context"
	"testing"
)

type recordingClient struct {
	calls int
}

func (c *recordingClient) StreamChat(context.Context, ChatRequest) (<-chan StreamEvent, error) {
	c.calls++
	events := make(chan StreamEvent)
	close(events)
	return events, nil
}

func TestWorkflowRoutesOnlySelectedOperations(t *testing.T) {
	legacy := &recordingClient{}
	eino := &recordingClient{}
	workflow := Workflow{
		Client:           legacy,
		OperationClients: map[string]AIClient{"organize": eino},
		Model:            "diary-default",
	}

	if _, err := workflow.Organize(context.Background(), "content", "structured"); err != nil {
		t.Fatal(err)
	}
	if _, err := workflow.GenerateReport(context.Background(), "report"); err != nil {
		t.Fatal(err)
	}
	if _, err := workflow.AnswerMemory(context.Background(), "question"); err != nil {
		t.Fatal(err)
	}
	if eino.calls != 1 || legacy.calls != 2 {
		t.Fatalf("calls: eino=%d legacy=%d", eino.calls, legacy.calls)
	}
}

func TestWorkflowCanRollbackAllOperationsToLegacy(t *testing.T) {
	legacy := &recordingClient{}
	workflow := Workflow{Client: legacy, Model: "diary-default"}

	_, _ = workflow.Organize(context.Background(), "content", "structured")
	_, _ = workflow.GenerateReport(context.Background(), "report")
	_, _ = workflow.AnswerMemory(context.Background(), "question")
	if legacy.calls != 3 {
		t.Fatalf("legacy calls = %d", legacy.calls)
	}
}
