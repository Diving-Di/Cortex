package tenant

import (
	"context"
	"strings"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
	"github.com/google/uuid"
)

// Repository is the persistence port required by tenant use cases. Its
// implementation remains responsible for transaction-local RLS context.
type Repository interface {
	GetTenant(context.Context, domain.Principal) (domain.TenantSummary, error)
	UpdateTenant(context.Context, domain.Principal, string) (domain.TenantSummary, error)
	DeleteTenant(context.Context, domain.Principal) ([]uuid.UUID, error)
}

// CacheInvalidator is a best-effort infrastructure port. PostgreSQL remains
// authoritative, so cache failure must not turn a committed deletion into an
// HTTP failure.
type CacheInvalidator interface {
	InvalidateDeletedTenant(context.Context, domain.Principal, []uuid.UUID)
}

type Service struct {
	repository Repository
	cache      CacheInvalidator
}

func NewService(repository Repository, cache CacheInvalidator) *Service {
	return &Service{repository: repository, cache: cache}
}

func (s *Service) Get(ctx context.Context, principal domain.Principal) (domain.TenantSummary, error) {
	return s.repository.GetTenant(ctx, principal)
}

func (s *Service) Update(ctx context.Context, principal domain.Principal, name string) (domain.TenantSummary, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.TenantSummary{}, apierror.New("TENANT_NAME_REQUIRED", "个人空间名称不能为空", 422)
	}
	return s.repository.UpdateTenant(ctx, principal, name)
}

func (s *Service) Delete(ctx context.Context, principal domain.Principal) error {
	publicTemplateIDs, err := s.repository.DeleteTenant(ctx, principal)
	if err != nil {
		return err
	}
	if s.cache != nil {
		s.cache.InvalidateDeletedTenant(ctx, principal, publicTemplateIDs)
	}
	return nil
}
