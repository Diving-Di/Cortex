package notes

import (
	"context"
	"strings"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
)

type Repository interface {
	ListNotes(context.Context, domain.Principal, domain.NoteFilter) ([]domain.Note, int64, error)
	CreateNote(context.Context, domain.Principal, domain.NoteInput) (domain.Note, error)
	GetNote(context.Context, domain.Principal, int32) (domain.Note, error)
	UpdateNote(context.Context, domain.Principal, int32, domain.NotePatch) (domain.Note, error)
	DeleteNote(context.Context, domain.Principal, int32) error
	ListRevisions(context.Context, domain.Principal, int32) ([]domain.Revision, error)
	RestoreRevision(context.Context, domain.Principal, int32, int32) (domain.Note, error)
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) List(ctx context.Context, p domain.Principal, filter domain.NoteFilter) ([]domain.Note, int64, error) {
	return s.repository.ListNotes(ctx, p, filter)
}

func (s *Service) Create(ctx context.Context, p domain.Principal, input domain.NoteInput) (domain.Note, error) {
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return domain.Note{}, apierror.New("TITLE_REQUIRED", "标题不能为空", 422)
	}
	return s.repository.CreateNote(ctx, p, input)
}

func (s *Service) Get(ctx context.Context, p domain.Principal, id int32) (domain.Note, error) {
	return s.repository.GetNote(ctx, p, id)
}

func (s *Service) Update(ctx context.Context, p domain.Principal, id int32, patch domain.NotePatch) (domain.Note, error) {
	return s.repository.UpdateNote(ctx, p, id, patch)
}

func (s *Service) Delete(ctx context.Context, p domain.Principal, id int32) error {
	return s.repository.DeleteNote(ctx, p, id)
}

func (s *Service) Revisions(ctx context.Context, p domain.Principal, id int32) ([]domain.Revision, error) {
	return s.repository.ListRevisions(ctx, p, id)
}

func (s *Service) Restore(ctx context.Context, p domain.Principal, noteID, revisionID int32) (domain.Note, error) {
	return s.repository.RestoreRevision(ctx, p, noteID, revisionID)
}
