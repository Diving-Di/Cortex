package reportjobs

import (
	"context"
	"strings"
	"time"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

type JobRepository interface {
	CreateReportJob(context.Context, domain.Principal, uuid.UUID, string, time.Time) (store.ReportJob, error)
	ListReportJobs(context.Context, domain.Principal) ([]store.ReportJob, error)
	GetReportJob(context.Context, domain.Principal, uuid.UUID) (store.ReportJob, error)
	CancelReportJob(context.Context, domain.Principal, uuid.UUID) error
	ConfirmReportJob(context.Context, domain.Principal, uuid.UUID, string, string, bool) (map[string]any, error)
	QueueScheduledReportJob(context.Context, domain.Principal, store.ScheduledTask, uuid.UUID, string, time.Time, time.Time) (store.ReportJob, error)
	ReleaseScheduledTaskLease(context.Context, domain.Principal, int32, uuid.UUID) error
}
type Service struct{ repository JobRepository }

func NewService(repository JobRepository) *Service { return &Service{repository: repository} }
func (s *Service) Create(ctx context.Context, p domain.Principal, requestID uuid.UUID, kind string, anchor time.Time) (store.ReportJob, error) {
	if requestID == uuid.Nil || (kind != "daily" && kind != "weekly" && kind != "monthly") {
		return store.ReportJob{}, apierror.Validation(nil)
	}
	return s.repository.CreateReportJob(ctx, p, requestID, kind, anchor)
}
func (s *Service) List(ctx context.Context, p domain.Principal) ([]store.ReportJob, error) {
	return s.repository.ListReportJobs(ctx, p)
}
func (s *Service) Get(ctx context.Context, p domain.Principal, id uuid.UUID) (store.ReportJob, error) {
	return s.repository.GetReportJob(ctx, p, id)
}
func (s *Service) Cancel(ctx context.Context, p domain.Principal, id uuid.UUID) error {
	return s.repository.CancelReportJob(ctx, p, id)
}
func (s *Service) Confirm(ctx context.Context, p domain.Principal, id uuid.UUID, title, content string, overwrite bool) (map[string]any, error) {
	job, err := s.repository.GetReportJob(ctx, p, id)
	if err != nil {
		return nil, err
	}
	if job.ConfirmedNoteID != nil {
		return map[string]any{"id": *job.ConfirmedNoteID}, nil
	}
	if err = ValidateConfirmation(title, content, job.Sources); err != nil {
		return nil, err
	}
	return s.repository.ConfirmReportJob(ctx, p, id, strings.TrimSpace(title), content, overwrite)
}
func (s *Service) QueueScheduled(ctx context.Context, p domain.Principal, task store.ScheduledTask, owner uuid.UUID, trigger string, anchor, next time.Time) (store.ReportJob, error) {
	return s.repository.QueueScheduledReportJob(ctx, p, task, owner, trigger, anchor, next)
}
func (s *Service) ReleaseSchedule(ctx context.Context, p domain.Principal, id int32, owner uuid.UUID) error {
	return s.repository.ReleaseScheduledTaskLease(ctx, p, id, owner)
}
