package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"diary-listener/backend/internal/ai"
	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/httpx"
	"diary-listener/backend/internal/store"
)

type aiChatRequest struct {
	Prompt string  `json:"prompt"`
	Model  *string `json:"model"`
}

type aiProviderRequest struct {
	DisplayName  string `json:"display_name"`
	BaseURL      string `json:"base_url"`
	DefaultModel string `json:"default_model"`
	Capabilities string `json:"capabilities"`
}

func (s *Server) aiSettings(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{
		"configured": s.cfg.AIAPIKey != "",
		"base_url":   s.cfg.AIBaseURL,
		"model":      s.cfg.AIModel,
		"api_key":    nil,
	})
}

func (s *Server) configureAIProvider(w http.ResponseWriter, r *http.Request) {
	var request aiProviderRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if request.Capabilities == "" {
		request.Capabilities = "chat,stream"
	}
	if strings.TrimSpace(request.DisplayName) == "" ||
		strings.TrimSpace(request.BaseURL) == "" ||
		strings.TrimSpace(request.DefaultModel) == "" {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	result, err := s.store.UpsertAIProvider(r.Context(), principalFrom(r.Context()), store.AIProvider{
		DisplayName: request.DisplayName, BaseURL: request.BaseURL,
		DefaultModel: request.DefaultModel, Capabilities: request.Capabilities,
	})
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (s *Server) streamAI(w http.ResponseWriter, r *http.Request) {
	var request aiChatRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if s.cfg.AIAPIKey == "" {
		httpx.WriteError(w, s.logger, apierror.New("AI_NOT_CONFIGURED", "AI 未配置，笔记功能仍可正常使用", 503))
		return
	}
	model := s.cfg.AIModel
	if request.Model != nil && strings.TrimSpace(*request.Model) != "" {
		model = strings.TrimSpace(*request.Model)
	}
	client := &ai.EinoClient{BaseURL: s.cfg.AIBaseURL, APIKey: s.cfg.AIAPIKey}
	aiCtx := s.aiContext(r.Context(), "chat", principalFrom(r.Context()))
	events, err := client.StreamChat(aiCtx, ai.ChatRequest{
		Model:    model,
		Messages: []ai.Message{{Role: "user", Content: request.Prompt}},
	})
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	s.writeSSE(w, r, "chat", model, request.Prompt, events, nil)
}

func (s *Server) writeSSE(
	w http.ResponseWriter,
	r *http.Request,
	requestType string,
	model string,
	prompt string,
	events <-chan ai.StreamEvent,
	after func(string) error,
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.WriteError(w, s.logger, errors.New("streaming unsupported"))
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	started := time.Now()
	var output strings.Builder
	status := "success"
	streamComplete := true
	var errorCode *string
	for event := range events {
		if event.Err != nil {
			status = "error"
			streamComplete = false
			value := "AI_REQUEST_FAILED"
			errorCode = &value
			payload, _ := json.Marshal(map[string]any{"code": value, "incomplete": output.Len() > 0, "output_tokens": len([]rune(output.String())) / 4, "upstream_stage": "generation"})
			_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", payload)
			flusher.Flush()
			break
		}
		output.WriteString(event.Content)
		payload, _ := json.Marshal(map[string]string{"content": event.Content})
		if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
			status = "cancelled"
			value := "CLIENT_DISCONNECTED"
			errorCode = &value
			break
		}
		flusher.Flush()
	}
	if status == "success" && after != nil {
		if err := after(output.String()); err != nil {
			status = "error"
			value := "AI_AFTER_HOOK_FAILED"
			errorCode = &value
			payload, _ := json.Marshal(map[string]string{"code": value})
			_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", payload)
		}
	}
	if status == "success" && streamComplete {
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}
	usageCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	if err := s.store.RecordAIUsage(usageCtx, principalFrom(r.Context()), store.AIUsage{
		RequestType: requestType, Model: model,
		InputTokens:  max(1, len([]rune(prompt))/4),
		OutputTokens: len([]rune(output.String())) / 4,
		Duration:     time.Since(started), Status: status, ErrorCode: errorCode,
	}); err != nil {
		s.logger.Error("record AI usage", "error", err)
	}
}
