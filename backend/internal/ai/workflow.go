package ai

import "context"

type Retriever interface {
    SearchNotes(ctx context.Context, query string, limit int) ([]NoteSource, error)
}

type NoteSource struct {
    ID       int32
    Title    string
    Content  string
    NoteDate string
}

type AIWorkflow interface {
    Organize(ctx context.Context, content, style string) (<-chan StreamEvent, error)
    GenerateReport(ctx context.Context, prompt string) (<-chan StreamEvent, error)
    AnswerMemory(ctx context.Context, prompt string) (<-chan StreamEvent, error)
}

type Workflow struct {
    Client AIClient
    Model  string
}

func (w Workflow) Organize(ctx context.Context, content, style string) (<-chan StreamEvent, error) {
    prompt := `你是中文笔记整理助手。不得添加原文没有的事实。请仅输出 JSON：{"title":"标题","summary":"摘要","content":"Markdown 正文"}。整理风格：` +
        style + "。原始记录：\n" + content
    return w.stream(ctx, prompt)
}

func (w Workflow) GenerateReport(ctx context.Context, prompt string) (<-chan StreamEvent, error) {
    return w.stream(ctx, prompt)
}

func (w Workflow) AnswerMemory(ctx context.Context, prompt string) (<-chan StreamEvent, error) {
    return w.stream(ctx, prompt)
}

func (w Workflow) stream(ctx context.Context, prompt string) (<-chan StreamEvent, error) {
    return w.Client.StreamChat(ctx, ChatRequest{
        Model:    w.Model,
        Messages: []Message{{Role: "user", Content: prompt}},
    })
}
