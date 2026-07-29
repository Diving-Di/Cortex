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
	Client           AIClient
	OperationClients map[string]AIClient
	Model            string
}

func (w Workflow) Organize(ctx context.Context, content, style string) (<-chan StreamEvent, error) {
	prompt := `你是中文笔记整理助手。不得添加原文没有的事实。请仅输出 JSON：{"title":"标题","summary":"摘要","content":"Markdown 正文"}。整理风格：` +
		style + "。原始记录：\n" + content
	return w.stream(ctx, "organize", prompt)
}

func (w Workflow) GenerateReport(ctx context.Context, prompt string) (<-chan StreamEvent, error) {
	return w.stream(ctx, "report", prompt)
}

func (w Workflow) AnswerMemory(ctx context.Context, prompt string) (<-chan StreamEvent, error) {
	return w.stream(ctx, "knowledge", prompt)
}

func (w Workflow) stream(ctx context.Context, operation, prompt string) (<-chan StreamEvent, error) {
	client := w.Client
	if operationClient := w.OperationClients[operation]; operationClient != nil {
		client = operationClient
	}
	return client.StreamChat(ctx, ChatRequest{
		Model:    w.Model,
		Messages: []Message{{Role: "user", Content: prompt}},
	})
}
