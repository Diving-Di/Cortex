package preferences

import (
	"context"

	"cortex/backend/internal/domain"
)

type Repository interface {
	GetUserPreferences(context.Context, domain.Principal) (domain.UserPreferences, error)
	UpdateUserPreferences(context.Context, domain.Principal, bool, int) (domain.UserPreferences, error)
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) Get(ctx context.Context, p domain.Principal) (domain.UserPreferences, error) {
	return s.repository.GetUserPreferences(ctx, p)
}
func (s *Service) Update(ctx context.Context, p domain.Principal, enabled bool, version int) (domain.UserPreferences, error) {
	return s.repository.UpdateUserPreferences(ctx, p, enabled, version)
}
