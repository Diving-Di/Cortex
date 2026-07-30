package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"diary-listener/backend/internal/ai"
	"diary-listener/backend/internal/domain"
	"diary-listener/backend/internal/store"
)

func (s *Server) writeRecipeSSE(
	w http.ResponseWriter,
	r *http.Request,
	prompt string,
	events <-chan ai.StreamEvent,
	sources []domain.Source,
	save func(context.Context, string) (int32, int32, error),
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	// A model response can legitimately take longer than the HTTP server's
	// ordinary WriteTimeout. Keep that protection for non-streaming handlers,
	// but allow this SSE response to remain open until the upstream stream ends.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_ = writeNamedSSE(w, "retrieval", map[string]any{"count": len(sources), "items": sources})
	flusher.Flush()
	started := time.Now()
	answer := ""
	status := "success"
	var errorCode *string
	for event := range events {
		if event.Err != nil {
			status = "error"
			code := "AI_REQUEST_FAILED"
			errorCode = &code
			_ = writeNamedSSE(w, "error", map[string]string{"code": code, "message": "生成服务暂时不可用"})
			flusher.Flush()
			break
		}
		answer += event.Content
		if err := writeNamedSSE(w, "delta", map[string]string{"content": event.Content}); err != nil {
			status = "cancelled"
			code := "CLIENT_DISCONNECTED"
			errorCode = &code
			break
		}
		flusher.Flush()
	}
	var messageID, conversationID int32
	if status == "success" {
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Second)
		var err error
		messageID, conversationID, err = save(persistCtx, answer)
		cancel()
		if err != nil {
			status = "error"
			code := "KNOWLEDGE_SOURCE_INVALID"
			errorCode = &code
			_ = writeNamedSSE(w, "error", map[string]string{"code": code, "message": "来源已失效，请重新提问"})
		} else {
			_ = writeNamedSSE(w, "sources", map[string]any{"items": sources})
			_ = writeNamedSSE(w, "done", map[string]any{
				"conversation_id": conversationID, "message_id": messageID,
			})
		}
		flusher.Flush()
	}
	usageCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	if err := s.store.RecordAIUsage(usageCtx, principalFrom(r.Context()), store.AIUsage{
		RequestType: "knowledge_chat", Model: s.cfg.AIModel,
		InputTokens: max(1, len([]rune(prompt))/4), OutputTokens: len([]rune(answer)) / 4,
		Duration: time.Since(started), Status: status, ErrorCode: errorCode,
		ConversationID: func() *int32 {
			if conversationID == 0 {
				return nil
			}
			return &conversationID
		}(),
	}); err != nil {
		s.logger.Error("record AI usage", "error", err)
	}
}

func writeNamedSSE(w http.ResponseWriter, event string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
	return err
}
