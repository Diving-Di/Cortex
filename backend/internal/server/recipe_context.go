package server

import (
	"fmt"
	"net/http"
	"strings"

	"diary-listener/backend/internal/domain"
)

func (s *Server) recipeConversationContext(
	r *http.Request,
	principal domain.Principal,
	conversationID *int32,
) string {
	if conversationID == nil {
		return ""
	}
	summary, _, messages, err := s.store.ConversationSummaryInput(
		r.Context(), principal, *conversationID,
	)
	if err != nil {
		return ""
	}
	if len(messages) > 10 {
		messages = messages[len(messages)-10:]
	}
	var result strings.Builder
	if summary != nil {
		result.WriteString("历史摘要：")
		result.WriteString(*summary)
		result.WriteByte('\n')
	}
	for _, message := range messages {
		_, _ = fmt.Fprintf(&result, "%s: %s\n", message.Role, message.Content)
	}
	return result.String()
}
