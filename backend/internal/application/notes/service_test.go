package notes

import (
	"context"
	"errors"
	"testing"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
)

type noteRepositoryStub struct {
	Repository
	input domain.NoteInput
}

func (r *noteRepositoryStub) CreateNote(_ context.Context, _ domain.Principal, input domain.NoteInput) (domain.Note, error) {
	r.input = input
	return domain.Note{Title: input.Title}, nil
}

func TestCreateNormalizesTitle(t *testing.T) {
	repository := &noteRepositoryStub{}
	result, err := NewService(repository).Create(context.Background(), domain.Principal{}, domain.NoteInput{Title: "  标题  "})
	if err != nil || repository.input.Title != "标题" || result.Title != "标题" {
		t.Fatalf("Create() result=%+v input=%+v err=%v", result, repository.input, err)
	}
}

func TestCreateRejectsBlankTitle(t *testing.T) {
	_, err := NewService(&noteRepositoryStub{}).Create(context.Background(), domain.Principal{}, domain.NoteInput{Title: "  "})
	var target *apierror.Error
	if !errors.As(err, &target) || target.Code != "TITLE_REQUIRED" {
		t.Fatalf("Create() error=%v", err)
	}
}
