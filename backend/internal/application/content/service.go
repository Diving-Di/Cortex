package content

import (
	"context"
	"strings"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
)

type Repository interface {
	ListTags(context.Context, domain.Principal) ([]domain.Tag, error)
	CreateTag(context.Context, domain.Principal, string, *string) (domain.Tag, error)
	ListNoteTags(context.Context, domain.Principal, int32) ([]domain.Tag, error)
	AssignNoteTags(context.Context, domain.Principal, int32, []int32) ([]domain.Tag, error)
	SearchNotes(context.Context, domain.Principal, domain.SearchFilter) ([]domain.SearchItem, int64, error)
	Dashboard(context.Context, domain.Principal, string) (map[string]any, error)
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) ListTags(ctx context.Context, p domain.Principal) ([]domain.Tag, error) {
	return s.repository.ListTags(ctx, p)
}
func (s *Service) CreateTag(ctx context.Context, p domain.Principal, name string, color *string) (domain.Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Tag{}, apierror.New("TAG_NAME_REQUIRED", "标签名称不能为空", 422)
	}
	return s.repository.CreateTag(ctx, p, name, color)
}
func (s *Service) ListNoteTags(ctx context.Context, p domain.Principal, noteID int32) ([]domain.Tag, error) {
	return s.repository.ListNoteTags(ctx, p, noteID)
}
func (s *Service) AssignNoteTags(ctx context.Context, p domain.Principal, noteID int32, ids []int32) ([]domain.Tag, error) {
	return s.repository.AssignNoteTags(ctx, p, noteID, ids)
}
func (s *Service) Search(ctx context.Context, p domain.Principal, filter domain.SearchFilter) ([]domain.SearchItem, int64, error) {
	return s.repository.SearchNotes(ctx, p, filter)
}
func (s *Service) Dashboard(ctx context.Context, p domain.Principal, timezone string) (map[string]any, error) {
	return s.repository.Dashboard(ctx, p, timezone)
}
