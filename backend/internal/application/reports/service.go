package reports

import (
	"context"
	"strings"
	"time"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

type Repository interface {
	ConfirmOrganize(context.Context, domain.Principal, *int32, string, string, *string) (map[string]any, error)
	ReportSources(context.Context, domain.Principal, string, time.Time) (time.Time, time.Time, []store.SourceNote, error)
	ConfirmReport(context.Context, domain.Principal, string, time.Time, string, string, []int32, bool, *uuid.UUID, int32) (map[string]any, error)
	GetReportSources(context.Context, domain.Principal, int32) ([]store.SourceNote, error)
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) ConfirmOrganize(ctx context.Context, p domain.Principal, noteID *int32, title, content string, summary *string) (map[string]any, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, apierror.New("TITLE_REQUIRED", "标题不能为空", 422)
	}
	return s.repository.ConfirmOrganize(ctx, p, noteID, title, content, summary)
}
func (s *Service) Sources(ctx context.Context, p domain.Principal, kind string, anchor time.Time) (time.Time, time.Time, []store.SourceNote, error) {
	return s.repository.ReportSources(ctx, p, kind, anchor)
}
func (s *Service) Confirm(ctx context.Context, p domain.Principal, kind string, anchor time.Time, title, content string, sourceIDs []int32, overwrite bool, owner *uuid.UUID, taskID int32) (map[string]any, error) {
	return s.repository.ConfirmReport(ctx, p, kind, anchor, title, content, sourceIDs, overwrite, owner, taskID)
}
func (s *Service) SavedSources(ctx context.Context, p domain.Principal, noteID int32) ([]store.SourceNote, error) {
	return s.repository.GetReportSources(ctx, p, noteID)
}
