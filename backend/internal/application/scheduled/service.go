package scheduled

import (
	"context"
	"time"

	"cortex/backend/internal/domain"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

type Repository interface {
	ClaimDueScheduledTasks(context.Context, uuid.UUID, int, time.Duration) ([]store.ClaimedScheduledTask, error)
	AcquireScheduledTaskLease(context.Context, domain.Principal, int32, uuid.UUID, time.Duration) error
	RenewScheduledTaskLease(context.Context, domain.Principal, int32, uuid.UUID, time.Duration) (bool, error)
	ListScheduledTasks(context.Context, domain.Principal) ([]store.ScheduledTask, error)
	CreateScheduledTask(context.Context, domain.Principal, string, int, int, string, time.Time) (store.ScheduledTask, error)
	SetScheduledTaskStatus(context.Context, domain.Principal, int32, bool, time.Time) (store.ScheduledTask, error)
	GetScheduledTask(context.Context, domain.Principal, int32) (store.ScheduledTask, error)
	ListScheduledRuns(context.Context, domain.Principal, int32) ([]store.ScheduledRun, error)
	StartScheduledRun(context.Context, domain.Principal, int32, string, uuid.UUID) (int64, error)
	FinishScheduledRun(context.Context, domain.Principal, store.ScheduledTask, int64, *int32, error, time.Time, uuid.UUID) error
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) Claim(c context.Context, o uuid.UUID, n int, d time.Duration) ([]store.ClaimedScheduledTask, error) {
	return s.repository.ClaimDueScheduledTasks(c, o, n, d)
}
func (s *Service) Acquire(c context.Context, p domain.Principal, id int32, o uuid.UUID, d time.Duration) error {
	return s.repository.AcquireScheduledTaskLease(c, p, id, o, d)
}
func (s *Service) Renew(c context.Context, p domain.Principal, id int32, o uuid.UUID, d time.Duration) (bool, error) {
	return s.repository.RenewScheduledTaskLease(c, p, id, o, d)
}
func (s *Service) List(c context.Context, p domain.Principal) ([]store.ScheduledTask, error) {
	return s.repository.ListScheduledTasks(c, p)
}
func (s *Service) Create(c context.Context, p domain.Principal, k string, h, m int, z string, n time.Time) (store.ScheduledTask, error) {
	return s.repository.CreateScheduledTask(c, p, k, h, m, z, n)
}
func (s *Service) SetStatus(c context.Context, p domain.Principal, id int32, e bool, n time.Time) (store.ScheduledTask, error) {
	return s.repository.SetScheduledTaskStatus(c, p, id, e, n)
}
func (s *Service) Get(c context.Context, p domain.Principal, id int32) (store.ScheduledTask, error) {
	return s.repository.GetScheduledTask(c, p, id)
}
func (s *Service) Runs(c context.Context, p domain.Principal, id int32) ([]store.ScheduledRun, error) {
	return s.repository.ListScheduledRuns(c, p, id)
}
func (s *Service) Start(c context.Context, p domain.Principal, id int32, t string, o uuid.UUID) (int64, error) {
	return s.repository.StartScheduledRun(c, p, id, t, o)
}
func (s *Service) Finish(c context.Context, p domain.Principal, t store.ScheduledTask, id int64, n *int32, e error, next time.Time, o uuid.UUID) error {
	return s.repository.FinishScheduledRun(c, p, t, id, n, e, next, o)
}
