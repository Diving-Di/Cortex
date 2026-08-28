package tenant

import (
	"context"
	"errors"
	"testing"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
	"github.com/google/uuid"
)

type repositoryStub struct {
	updatedName string
	deleteIDs   []uuid.UUID
	deleteErr   error
}

func (r *repositoryStub) GetTenant(context.Context, domain.Principal) (domain.TenantSummary, error) {
	return domain.TenantSummary{Name: "个人空间"}, nil
}

func (r *repositoryStub) UpdateTenant(_ context.Context, _ domain.Principal, name string) (domain.TenantSummary, error) {
	r.updatedName = name
	return domain.TenantSummary{Name: name}, nil
}

func (r *repositoryStub) DeleteTenant(context.Context, domain.Principal) ([]uuid.UUID, error) {
	return r.deleteIDs, r.deleteErr
}

type cacheStub struct {
	called bool
	ids    []uuid.UUID
}

func (c *cacheStub) InvalidateDeletedTenant(_ context.Context, _ domain.Principal, ids []uuid.UUID) {
	c.called = true
	c.ids = ids
}

func TestUpdateTrimsNameBeforeRepository(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository, nil)

	result, err := service.Update(context.Background(), domain.Principal{}, "  新空间  ")
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if repository.updatedName != "新空间" || result.Name != "新空间" {
		t.Fatalf("trimmed name = %q, result = %q", repository.updatedName, result.Name)
	}
}

func TestUpdateRejectsBlankNameWithoutRepositoryCall(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository, nil)

	_, err := service.Update(context.Background(), domain.Principal{}, " \t ")
	var target *apierror.Error
	if !errors.As(err, &target) || target.Code != "TENANT_NAME_REQUIRED" {
		t.Fatalf("Update() error = %v", err)
	}
	if repository.updatedName != "" {
		t.Fatalf("repository called with %q", repository.updatedName)
	}
}

func TestDeleteInvalidatesCacheAfterCommit(t *testing.T) {
	id := uuid.New()
	repository := &repositoryStub{deleteIDs: []uuid.UUID{id}}
	cache := &cacheStub{}
	service := NewService(repository, cache)

	if err := service.Delete(context.Background(), domain.Principal{}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !cache.called || len(cache.ids) != 1 || cache.ids[0] != id {
		t.Fatalf("cache invalidation = called %v, ids %v", cache.called, cache.ids)
	}
}

func TestDeleteDoesNotInvalidateCacheWhenRepositoryFails(t *testing.T) {
	repository := &repositoryStub{deleteErr: errors.New("database unavailable")}
	cache := &cacheStub{}
	service := NewService(repository, cache)

	if err := service.Delete(context.Background(), domain.Principal{}); err == nil {
		t.Fatal("Delete() error = nil")
	}
	if cache.called {
		t.Fatal("cache invalidated after repository failure")
	}
}
