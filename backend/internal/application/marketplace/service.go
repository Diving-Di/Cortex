package marketplace

import (
	"context"
	"time"

	"cortex/backend/internal/domain"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

type Repository interface {
	GetPublicProfile(context.Context, domain.Principal) (store.PublicProfile, error)
	UpsertPublicProfile(context.Context, domain.Principal, string, bool, *int) (store.PublicProfile, error)
	ListWritingTemplates(context.Context, domain.Principal) ([]store.WritingTemplate, error)
	CreateWritingTemplate(context.Context, domain.Principal, store.TemplateInput) (store.WritingTemplate, error)
	GetWritingTemplate(context.Context, domain.Principal, int64) (store.WritingTemplate, error)
	UpdateWritingTemplate(context.Context, domain.Principal, int64, store.TemplateInput, int) (store.WritingTemplate, error)
	DeleteWritingTemplate(context.Context, domain.Principal, int64) error
	PublishWritingTemplate(context.Context, domain.Principal, int64) (store.PublicTemplate, error)
	WithdrawWritingTemplate(context.Context, domain.Principal, int64) error
	GetPublicTemplatesByIDs(context.Context, domain.Principal, []uuid.UUID) ([]store.PublicTemplate, error)
	ListPublicTemplates(context.Context, domain.Principal, string, string, string, int, *float64, *uuid.UUID) ([]store.PublicTemplate, []float64, error)
	GetPublicTemplateVersion(context.Context, domain.Principal, uuid.UUID) (int, error)
	GetPublicTemplate(context.Context, domain.Principal, uuid.UUID) (store.PublicTemplate, error)
	GetTemplateReactions(context.Context, domain.Principal, uuid.UUID) (bool, bool, error)
	SetTemplateReaction(context.Context, domain.Principal, uuid.UUID, string, bool) error
	UsePublicTemplate(context.Context, domain.Principal, uuid.UUID, string) (int32, error)
	UseWritingTemplate(context.Context, domain.Principal, int64, string) (int32, error)
	RecordTemplateView(context.Context, domain.Principal, uuid.UUID) error
	ListTemplateRankingProjections(context.Context) ([]store.TemplateRankingProjection, error)
	MarkTemplateOutboxRebuilt(context.Context, time.Time) error
	ClaimOutboxEvent(context.Context, string, string, time.Duration) (*store.OutboxEvent, error)
	RenewOutboxEventLease(context.Context, string, string, time.Duration) (bool, error)
	GetTemplateEventProjection(context.Context, string) (store.TemplateEventProjection, error)
	FinishOutboxEvent(context.Context, string, string, error) error
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) Profile(ctx context.Context, p domain.Principal) (store.PublicProfile, error) {
	return s.repository.GetPublicProfile(ctx, p)
}
func (s *Service) SaveProfile(ctx context.Context, p domain.Principal, n string, d bool, v *int) (store.PublicProfile, error) {
	return s.repository.UpsertPublicProfile(ctx, p, n, d, v)
}
func (s *Service) Writing(ctx context.Context, p domain.Principal) ([]store.WritingTemplate, error) {
	return s.repository.ListWritingTemplates(ctx, p)
}
func (s *Service) CreateWriting(ctx context.Context, p domain.Principal, i store.TemplateInput) (store.WritingTemplate, error) {
	return s.repository.CreateWritingTemplate(ctx, p, i)
}
func (s *Service) WritingByID(ctx context.Context, p domain.Principal, id int64) (store.WritingTemplate, error) {
	return s.repository.GetWritingTemplate(ctx, p, id)
}
func (s *Service) UpdateWriting(ctx context.Context, p domain.Principal, id int64, i store.TemplateInput, v int) (store.WritingTemplate, error) {
	return s.repository.UpdateWritingTemplate(ctx, p, id, i, v)
}
func (s *Service) DeleteWriting(ctx context.Context, p domain.Principal, id int64) error {
	return s.repository.DeleteWritingTemplate(ctx, p, id)
}
func (s *Service) Publish(ctx context.Context, p domain.Principal, id int64) (store.PublicTemplate, error) {
	return s.repository.PublishWritingTemplate(ctx, p, id)
}
func (s *Service) Withdraw(ctx context.Context, p domain.Principal, id int64) error {
	return s.repository.WithdrawWritingTemplate(ctx, p, id)
}
func (s *Service) PublicByIDs(ctx context.Context, p domain.Principal, ids []uuid.UUID) ([]store.PublicTemplate, error) {
	return s.repository.GetPublicTemplatesByIDs(ctx, p, ids)
}
func (s *Service) Public(ctx context.Context, p domain.Principal, q, c, r string, l int, score *float64, id *uuid.UUID) ([]store.PublicTemplate, []float64, error) {
	return s.repository.ListPublicTemplates(ctx, p, q, c, r, l, score, id)
}
func (s *Service) PublicVersion(ctx context.Context, p domain.Principal, id uuid.UUID) (int, error) {
	return s.repository.GetPublicTemplateVersion(ctx, p, id)
}
func (s *Service) PublicByID(ctx context.Context, p domain.Principal, id uuid.UUID) (store.PublicTemplate, error) {
	return s.repository.GetPublicTemplate(ctx, p, id)
}
func (s *Service) Reactions(ctx context.Context, p domain.Principal, id uuid.UUID) (bool, bool, error) {
	return s.repository.GetTemplateReactions(ctx, p, id)
}
func (s *Service) SetReaction(ctx context.Context, p domain.Principal, id uuid.UUID, k string, e bool) error {
	return s.repository.SetTemplateReaction(ctx, p, id, k, e)
}
func (s *Service) UsePublic(ctx context.Context, p domain.Principal, id uuid.UUID, k string) (int32, error) {
	return s.repository.UsePublicTemplate(ctx, p, id, k)
}
func (s *Service) UseWriting(ctx context.Context, p domain.Principal, id int64, k string) (int32, error) {
	return s.repository.UseWritingTemplate(ctx, p, id, k)
}
func (s *Service) View(ctx context.Context, p domain.Principal, id uuid.UUID) error {
	return s.repository.RecordTemplateView(ctx, p, id)
}
func (s *Service) Rankings(ctx context.Context) ([]store.TemplateRankingProjection, error) {
	return s.repository.ListTemplateRankingProjections(ctx)
}
func (s *Service) MarkRebuilt(ctx context.Context, at time.Time) error {
	return s.repository.MarkTemplateOutboxRebuilt(ctx, at)
}
func (s *Service) ClaimEvent(ctx context.Context, aggregate, owner string, lease time.Duration) (*store.OutboxEvent, error) {
	return s.repository.ClaimOutboxEvent(ctx, aggregate, owner, lease)
}
func (s *Service) RenewEvent(ctx context.Context, id, owner string, lease time.Duration) (bool, error) {
	return s.repository.RenewOutboxEventLease(ctx, id, owner, lease)
}
func (s *Service) Projection(ctx context.Context, id string) (store.TemplateEventProjection, error) {
	return s.repository.GetTemplateEventProjection(ctx, id)
}
func (s *Service) FinishEvent(ctx context.Context, id, owner string, err error) error {
	return s.repository.FinishOutboxEvent(ctx, id, owner, err)
}
