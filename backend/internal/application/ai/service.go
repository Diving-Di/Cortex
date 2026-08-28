package ai

import (
	"context"
	"strings"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
)

type Repository interface {
	UpsertAIProvider(context.Context, domain.Principal, domain.AIProvider) (domain.AIProvider, error)
	RecordAIUsage(context.Context, domain.Principal, domain.AIUsage) error
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) Configure(ctx context.Context, p domain.Principal, input domain.AIProvider) (domain.AIProvider, error) {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.BaseURL = strings.TrimSpace(input.BaseURL)
	input.DefaultModel = strings.TrimSpace(input.DefaultModel)
	if input.Capabilities == "" {
		input.Capabilities = "chat,stream"
	}
	if input.DisplayName == "" || input.BaseURL == "" || input.DefaultModel == "" {
		return domain.AIProvider{}, apierror.Validation(nil)
	}
	return s.repository.UpsertAIProvider(ctx, p, input)
}
func (s *Service) RecordUsage(ctx context.Context, p domain.Principal, usage domain.AIUsage) error {
	return s.repository.RecordAIUsage(ctx, p, usage)
}
