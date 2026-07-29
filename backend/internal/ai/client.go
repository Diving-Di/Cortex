package ai

import "context"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string
	Messages []Message
}

type StreamEvent struct {
	Content string
	Err     error
}

// AIClient is the stable streaming boundary used by non-workflow callers.
// The production implementation is EinoClient.
type AIClient interface {
	StreamChat(ctx context.Context, request ChatRequest) (<-chan StreamEvent, error)
}
