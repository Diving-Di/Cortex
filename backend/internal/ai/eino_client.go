package ai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"diary-listener/backend/internal/apierror"
	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// EinoClient adapts Eino's OpenAI-compatible ChatModel to the stable AIClient
// contract. BaseURL must continue to point at LiteLLM; this adapter never
// selects or contacts a model vendor directly.
type EinoClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Model      model.BaseChatModel
}

func (c *EinoClient) StreamChat(ctx context.Context, request ChatRequest) (<-chan StreamEvent, error) {
	if c.Model == nil && strings.TrimSpace(c.APIKey) == "" {
		return nil, errors.New("AI_NOT_CONFIGURED")
	}
	chatModel := c.Model
	if chatModel == nil {
		httpClient := c.HTTPClient
		if httpClient == nil {
			httpClient = &http.Client{Timeout: 60 * time.Second}
		}
		created, err := einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{
			APIKey: c.APIKey, BaseURL: c.BaseURL, Model: request.Model, HTTPClient: httpClient,
		})
		if err != nil {
			return nil, stableEinoError(ctx, err)
		}
		chatModel = created
	}
	messages := make([]*schema.Message, 0, len(request.Messages))
	for _, item := range request.Messages {
		role := schema.RoleType(item.Role)
		switch role {
		case schema.System, schema.User, schema.Assistant:
		default:
			role = schema.User
		}
		messages = append(messages, &schema.Message{Role: role, Content: item.Content})
	}
	metadata := requestMetadataFrom(ctx)
	fields := map[string]any{}
	headers := map[string]string{}
	if metadata.RequestID != "" {
		fields["metadata"] = map[string]string{
			"request_id": metadata.RequestID, "request_type": metadata.RequestType,
			"tenant": metadata.Tenant, "environment": metadata.Environment,
		}
		headers["X-Request-ID"] = metadata.RequestID
	}
	options := make([]model.Option, 0, 2)
	if len(fields) > 0 {
		options = append(options, einoopenai.WithExtraFields(fields))
	}
	if len(headers) > 0 {
		options = append(options, einoopenai.WithExtraHeader(headers))
	}
	reader, err := chatModel.Stream(ctx, messages, options...)
	if err != nil {
		return nil, stableEinoError(ctx, err)
	}
	events := make(chan StreamEvent)
	go func() {
		defer close(events)
		defer reader.Close()
		for {
			message, receiveErr := reader.Recv()
			if errors.Is(receiveErr, io.EOF) {
				return
			}
			if receiveErr != nil {
				select {
				case events <- StreamEvent{Err: stableEinoError(ctx, receiveErr)}:
				case <-ctx.Done():
				}
				return
			}
			if message == nil || message.Content == "" {
				continue
			}
			select {
			case events <- StreamEvent{Content: message.Content}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return events, nil
}

func stableEinoError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	value := strings.ToLower(err.Error())
	switch {
	case strings.Contains(value, "401"), strings.Contains(value, "403"), strings.Contains(value, "unauthorized"):
		return stableGatewayError("AI_GATEWAY_AUTH_FAILED", "AI 网关认证失败，请联系管理员更新服务凭据")
	case strings.Contains(value, "429"), strings.Contains(value, "rate limit"):
		return stableGatewayError("AI_RATE_LIMITED", "AI 服务繁忙，请稍后重试")
	default:
		return stableGatewayError("AI_UPSTREAM_UNAVAILABLE", "AI 服务暂时不可用")
	}
}

func stableGatewayError(code, message string) error {
	return apierror.New(code, message, 503)
}
