package ai

import (
    "bufio"
    "bytes"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "strings"
    "time"
)

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

type AIClient interface {
    StreamChat(ctx context.Context, request ChatRequest) (<-chan StreamEvent, error)
}

type OpenAICompatibleClient struct {
    BaseURL    string
    APIKey     string
    HTTPClient *http.Client
}

func (c *OpenAICompatibleClient) StreamChat(ctx context.Context, request ChatRequest) (<-chan StreamEvent, error) {
    if strings.TrimSpace(c.APIKey) == "" {
        return nil, errors.New("AI_NOT_CONFIGURED")
    }
    payload, err := json.Marshal(map[string]any{
        "model": request.Model, "messages": request.Messages, "stream": true,
    })
    if err != nil {
        return nil, err
    }
    httpClient := c.HTTPClient
    if httpClient == nil {
        httpClient = &http.Client{Timeout: 60 * time.Second}
    }
    endpoint := strings.TrimRight(c.BaseURL, "/") + "/chat/completions"
    response, err := requestWithRetry(ctx, httpClient, endpoint, c.APIKey, payload)
    if err != nil {
        return nil, err
    }
    events := make(chan StreamEvent)
    go func() {
        defer close(events)
        defer response.Body.Close()
        scanner := bufio.NewScanner(response.Body)
        scanner.Buffer(make([]byte, 64*1024), 1024*1024)
        for scanner.Scan() {
            line := scanner.Text()
            if !strings.HasPrefix(line, "data:") {
                continue
            }
            raw := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
            if raw == "[DONE]" {
                return
            }
            var value struct {
                Choices []struct {
                    Delta struct {
                        Content string `json:"content"`
                    } `json:"delta"`
                } `json:"choices"`
            }
            if err := json.Unmarshal([]byte(raw), &value); err != nil {
                events <- StreamEvent{Err: fmt.Errorf("decode AI stream: %w", err)}
                return
            }
            if len(value.Choices) > 0 && value.Choices[0].Delta.Content != "" {
                select {
                case events <- StreamEvent{Content: value.Choices[0].Delta.Content}:
                case <-ctx.Done():
                    return
                }
            }
        }
        if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
            events <- StreamEvent{Err: err}
        }
    }()
    return events, nil
}

func requestWithRetry(
    ctx context.Context,
    client *http.Client,
    endpoint string,
    apiKey string,
    payload []byte,
) (*http.Response, error) {
    var lastErr error
    for attempt := 0; attempt < 2; attempt++ {
        request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
        if err != nil {
            return nil, err
        }
        request.Header.Set("Authorization", "Bearer "+apiKey)
        request.Header.Set("Content-Type", "application/json")
        response, err := client.Do(request)
        if err != nil {
            lastErr = err
            if attempt == 0 {
                select {
                case <-time.After(250 * time.Millisecond):
                    continue
                case <-ctx.Done():
                    return nil, ctx.Err()
                }
            }
            return nil, err
        }
        if response.StatusCode < 200 || response.StatusCode >= 300 {
            detail, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
            response.Body.Close()
            return nil, fmt.Errorf("AI upstream HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(detail)))
        }
        return response, nil
    }
    return nil, lastErr
}
