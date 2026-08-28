package content

import (
	"context"
	"errors"
	"testing"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
)

type contentRepositoryStub struct {
	Repository
	name string
}

func (r *contentRepositoryStub) CreateTag(_ context.Context, _ domain.Principal, name string, _ *string) (domain.Tag, error) {
	r.name = name
	return domain.Tag{Name: name}, nil
}

func TestCreateTagNormalizesAndValidatesName(t *testing.T) {
	repository := &contentRepositoryStub{}
	result, err := NewService(repository).CreateTag(context.Background(), domain.Principal{}, " 标签 ", nil)
	if err != nil || result.Name != "标签" || repository.name != "标签" {
		t.Fatalf("CreateTag() result=%+v name=%q err=%v", result, repository.name, err)
	}
	_, err = NewService(repository).CreateTag(context.Background(), domain.Principal{}, " ", nil)
	var target *apierror.Error
	if !errors.As(err, &target) || target.Code != "TAG_NAME_REQUIRED" {
		t.Fatalf("CreateTag(blank) error=%v", err)
	}
}
