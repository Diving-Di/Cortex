package research

import (
	"context"
	"encoding/json"
	"time"

	"cortex/backend/internal/domain"
	researchcore "cortex/backend/internal/research"
	"cortex/backend/internal/store"
)

type Repository interface {
	CreateResearchJob(context.Context, domain.Principal, string, json.RawMessage, int, string, int) (store.ResearchJob, error)
	ListResearchJobs(context.Context, domain.Principal, int, int) ([]store.ResearchJob, int64, error)
	GetResearchJob(context.Context, domain.Principal, int64) (store.ResearchJob, error)
	CancelResearchJob(context.Context, domain.Principal, int64) error
	RetryResearchJob(context.Context, domain.Principal, int64) error
	ListResearchSources(context.Context, domain.Principal, store.ResearchSourceFilter) ([]store.ResearchSource, int64, error)
	GetResearchSource(context.Context, domain.Principal, int64) (store.ResearchSource, error)
	UpdateResearchDraft(context.Context, domain.Principal, int64, int, string, []string, []string, string) (store.ResearchDraft, error)
	IgnoreResearchSources(context.Context, domain.Principal, []int64) error
	GetResearchAsset(context.Context, domain.Principal, int64) (store.ResearchAsset, error)
	SoftDeleteResearchSource(context.Context, domain.Principal, int64) error
	ClaimResearchJobs(context.Context, string, int, time.Duration) ([]store.ResearchJob, error)
	AddResearchSource(context.Context, domain.Principal, int64, string, string) (store.ResearchSource, error)
	CompleteResearchSource(context.Context, domain.Principal, int64, string, string, string, string, string, []string, *time.Time, int64, int64, int64, string, []string, string, []string, string, researchcore.ContentDiagnostics) error
	FailResearchSource(context.Context, domain.Principal, int64, string, string) error
	CompleteResearchJob(context.Context, domain.Principal, int64, bool, string) error
	SetResearchJobStage(context.Context, domain.Principal, int64, string, time.Duration) error
	RequeueResearchJob(context.Context, domain.Principal, int64, time.Duration) error
	DeferResearchJob(context.Context, domain.Principal, int64, time.Duration, string) error
	AddResearchAsset(context.Context, domain.Principal, int64, int, string, string, string, int64, string, string, string) (store.ResearchAsset, error)
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) Create(ctx context.Context, p domain.Principal, m string, q json.RawMessage, n int, k string, a int) (store.ResearchJob, error) {
	return s.repository.CreateResearchJob(ctx, p, m, q, n, k, a)
}
func (s *Service) Jobs(ctx context.Context, p domain.Principal, l, o int) ([]store.ResearchJob, int64, error) {
	return s.repository.ListResearchJobs(ctx, p, l, o)
}
func (s *Service) Job(ctx context.Context, p domain.Principal, id int64) (store.ResearchJob, error) {
	return s.repository.GetResearchJob(ctx, p, id)
}
func (s *Service) Cancel(ctx context.Context, p domain.Principal, id int64) error {
	return s.repository.CancelResearchJob(ctx, p, id)
}
func (s *Service) Retry(ctx context.Context, p domain.Principal, id int64) error {
	return s.repository.RetryResearchJob(ctx, p, id)
}
func (s *Service) Sources(ctx context.Context, p domain.Principal, f store.ResearchSourceFilter) ([]store.ResearchSource, int64, error) {
	return s.repository.ListResearchSources(ctx, p, f)
}
func (s *Service) Source(ctx context.Context, p domain.Principal, id int64) (store.ResearchSource, error) {
	return s.repository.GetResearchSource(ctx, p, id)
}
func (s *Service) Draft(ctx context.Context, p domain.Principal, id int64, v int, summary string, points, tags []string, category string) (store.ResearchDraft, error) {
	return s.repository.UpdateResearchDraft(ctx, p, id, v, summary, points, tags, category)
}
func (s *Service) Ignore(ctx context.Context, p domain.Principal, ids []int64) error {
	return s.repository.IgnoreResearchSources(ctx, p, ids)
}
func (s *Service) Asset(ctx context.Context, p domain.Principal, id int64) (store.ResearchAsset, error) {
	return s.repository.GetResearchAsset(ctx, p, id)
}
func (s *Service) DeleteSource(ctx context.Context, p domain.Principal, id int64) error {
	return s.repository.SoftDeleteResearchSource(ctx, p, id)
}
func (s *Service) ClaimResearchJobs(c context.Context, o string, n int, d time.Duration) ([]store.ResearchJob, error) {
	return s.repository.ClaimResearchJobs(c, o, n, d)
}
func (s *Service) AddResearchSource(c context.Context, p domain.Principal, j int64, u, n string) (store.ResearchSource, error) {
	return s.repository.AddResearchSource(c, p, j, u, n)
}
func (s *Service) CompleteResearchSource(c context.Context, p domain.Principal, id int64, title, author, content, formatted, hash string, tags []string, published *time.Time, likes, collects, comments int64, summary string, points []string, category string, suggested []string, model string, d researchcore.ContentDiagnostics) error {
	return s.repository.CompleteResearchSource(c, p, id, title, author, content, formatted, hash, tags, published, likes, collects, comments, summary, points, category, suggested, model, d)
}
func (s *Service) FailResearchSource(c context.Context, p domain.Principal, id int64, code, summary string) error {
	return s.repository.FailResearchSource(c, p, id, code, summary)
}
func (s *Service) CompleteResearchJob(c context.Context, p domain.Principal, id int64, failed bool, code string) error {
	return s.repository.CompleteResearchJob(c, p, id, failed, code)
}
func (s *Service) SetResearchJobStage(c context.Context, p domain.Principal, id int64, status string, d time.Duration) error {
	return s.repository.SetResearchJobStage(c, p, id, status, d)
}
func (s *Service) RequeueResearchJob(c context.Context, p domain.Principal, id int64, d time.Duration) error {
	return s.repository.RequeueResearchJob(c, p, id, d)
}
func (s *Service) DeferResearchJob(c context.Context, p domain.Principal, id int64, d time.Duration, code string) error {
	return s.repository.DeferResearchJob(c, p, id, d, code)
}
func (s *Service) AddResearchAsset(c context.Context, p domain.Principal, id int64, pos int, path, url, mime string, size int64, digest, status, text string) (store.ResearchAsset, error) {
	return s.repository.AddResearchAsset(c, p, id, pos, path, url, mime, size, digest, status, text)
}
func (s *Service) GetResearchJob(c context.Context, p domain.Principal, id int64) (store.ResearchJob, error) {
	return s.repository.GetResearchJob(c, p, id)
}
