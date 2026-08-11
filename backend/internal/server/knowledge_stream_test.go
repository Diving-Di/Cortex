package server

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"diary-listener/backend/internal/ai"
)

func TestWriteKnowledgeSSEPersistsPartialFailureWithoutDone(t *testing.T) {
	events := make(chan ai.StreamEvent, 2)
	events <- ai.StreamEvent{Content: "部分回答"}
	events <- ai.StreamEvent{Err: errors.New("upstream reset")}
	close(events)

	var savedStatus, savedContent, savedStage string
	var savedTokens int
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/knowledge/chat/stream", nil)
	(&Server{}).writeKnowledgeSSE(w, r, events, nil, func(_ context.Context, answer, status, errorCode, stage string, outputTokens int) (int32, int32, error) {
		savedContent, savedStatus, savedStage, savedTokens = answer, status, stage, outputTokens
		if errorCode != "AI_REQUEST_FAILED" {
			t.Fatalf("unexpected error code %q", errorCode)
		}
		return 7, 9, nil
	})

	body := w.Body.String()
	if strings.Contains(body, "event: done") || strings.Contains(body, "[DONE]") {
		t.Fatalf("failed stream emitted completion marker: %s", body)
	}
	if !strings.Contains(body, `"incomplete":true`) || !strings.Contains(body, `"upstream_stage":"generation"`) {
		t.Fatalf("missing incomplete diagnostics: %s", body)
	}
	if savedStatus != "failed" || savedContent != "部分回答" || savedStage != "generation" || savedTokens < 1 {
		t.Fatalf("unexpected persisted state: status=%q content=%q stage=%q tokens=%d", savedStatus, savedContent, savedStage, savedTokens)
	}
}
