package conversations

import (
	"context"
	"strings"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
)

type Repository interface {
	CreateConversation(context.Context, domain.Principal, string, string) (domain.Conversation, error)
	ListScopedConversations(context.Context, domain.Principal, string, string, int, int) ([]domain.Conversation, int, error)
	RenameConversation(context.Context, domain.Principal, int32, string, int) (domain.Conversation, error)
	GetConversation(context.Context, domain.Principal, int32) (domain.Conversation, error)
	DeleteConversation(context.Context, domain.Principal, int32) error
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }
func validScope(scope string) bool {
	return scope == "knowledge" || scope == "growth" || scope == "all"
}
func (s *Service) List(ctx context.Context, p domain.Principal, search, scope string, limit, offset int) ([]domain.Conversation, int, error) {
	if scope != "" && !validScope(scope) {
		return nil, 0, apierror.Validation(nil)
	}
	return s.repository.ListScopedConversations(ctx, p, strings.TrimSpace(search), scope, limit, offset)
}
func (s *Service) Create(ctx context.Context, p domain.Principal, title, scope string) (domain.Conversation, error) {
	title, scope = strings.TrimSpace(title), strings.TrimSpace(scope)
	if title == "" {
		title = "新对话"
	}
	if len([]rune(title)) > 80 || !validScope(scope) {
		return domain.Conversation{}, apierror.Validation(nil)
	}
	return s.repository.CreateConversation(ctx, p, title, scope)
}
func (s *Service) Rename(ctx context.Context, p domain.Principal, id int32, title string, version int) (domain.Conversation, error) {
	title = strings.TrimSpace(title)
	if len([]rune(title)) < 1 || len([]rune(title)) > 255 || version < 1 {
		return domain.Conversation{}, apierror.Validation(nil)
	}
	return s.repository.RenameConversation(ctx, p, id, title, version)
}
func (s *Service) Get(ctx context.Context, p domain.Principal, id int32) (domain.Conversation, error) {
	result, err := s.repository.GetConversation(ctx, p, id)
	if err == nil && !validScope(result.SourceScope) {
		err = apierror.New("CONVERSATION_NOT_FOUND", "对话不存在", 404)
	}
	return result, err
}
func (s *Service) Delete(ctx context.Context, p domain.Principal, id int32) error {
	if _, err := s.Get(ctx, p, id); err != nil {
		return err
	}
	return s.repository.DeleteConversation(ctx, p, id)
}
