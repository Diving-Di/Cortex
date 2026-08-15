package server

import (
	"strings"
	"testing"

	"cortex/backend/internal/store"
)

func TestFormatKnowledgeConversationKeepsRecentCompleteTurnsWithinBudget(t *testing.T) {
	messages := []store.KnowledgeConversationMessage{
		{Role: "user", Content: "旧问题"},
		{Role: "assistant", Content: strings.Repeat("旧", 40)},
		{Role: "user", Content: "租约多久？"},
		{Role: "assistant", Content: "五分钟"},
	}
	got := formatKnowledgeConversation(messages, 30)
	if strings.Contains(got, "旧问题") || got != "用户：租约多久？\n助手：五分钟" {
		t.Fatalf("context=%q", got)
	}
}

func TestFormatKnowledgeConversationRejectsIncompletePairs(t *testing.T) {
	messages := []store.KnowledgeConversationMessage{{Role: "user", Content: "没有成功回答"}}
	if got := formatKnowledgeConversation(messages, 100); got != "" {
		t.Fatalf("context=%q", got)
	}
}
