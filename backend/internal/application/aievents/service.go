package aievents

import (
	"context"
	"time"

	"cortex/backend/internal/domain"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

type Repository interface {
	EnsureDailyAIEvent(context.Context, time.Time) error
	GetAIPointBalance(context.Context, domain.Principal) (store.AIPointBalance, error)
	GetCurrentAIEvent(context.Context, domain.Principal) (store.AIFlashEvent, error)
	GetAIEventPage(context.Context, domain.Principal) (store.AIEventPage, error)
	ListAIEventHistory(context.Context) ([]store.AIEventHistoryItem, error)
	GetAIEvent(context.Context, domain.Principal, uuid.UUID) (store.AIFlashEvent, error)
	GetMyAIEventClaim(context.Context, domain.Principal, uuid.UUID) (store.AIFlashClaim, error)
	ClaimAIEventReserved(context.Context, domain.Principal, uuid.UUID, uuid.UUID, string) (store.AIFlashClaim, error)
	ClaimAIEventFallback(context.Context, domain.Principal, uuid.UUID, uuid.UUID) (store.AIFlashClaim, error)
	GetAIEventClaim(context.Context, domain.Principal, int64) (store.AIFlashClaim, error)
	ReconcileAIEventClaimCounts(context.Context) error
	SetAIEventReservationReady(context.Context, uuid.UUID, bool) error
	GetAIEventReservationState(context.Context) (store.AIEventReservationState, error)
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) Ensure(ctx context.Context, now time.Time) error {
	return s.repository.EnsureDailyAIEvent(ctx, now)
}
func (s *Service) Balance(ctx context.Context, p domain.Principal) (store.AIPointBalance, error) {
	return s.repository.GetAIPointBalance(ctx, p)
}
func (s *Service) Current(ctx context.Context, p domain.Principal) (store.AIFlashEvent, error) {
	return s.repository.GetCurrentAIEvent(ctx, p)
}
func (s *Service) Page(ctx context.Context, p domain.Principal) (store.AIEventPage, error) {
	return s.repository.GetAIEventPage(ctx, p)
}
func (s *Service) History(ctx context.Context) ([]store.AIEventHistoryItem, error) {
	return s.repository.ListAIEventHistory(ctx)
}
func (s *Service) Event(ctx context.Context, p domain.Principal, id uuid.UUID) (store.AIFlashEvent, error) {
	return s.repository.GetAIEvent(ctx, p, id)
}
func (s *Service) MyClaim(ctx context.Context, p domain.Principal, id uuid.UUID) (store.AIFlashClaim, error) {
	return s.repository.GetMyAIEventClaim(ctx, p, id)
}
func (s *Service) ClaimReserved(ctx context.Context, p domain.Principal, eventID, requestID uuid.UUID, version string) (store.AIFlashClaim, error) {
	return s.repository.ClaimAIEventReserved(ctx, p, eventID, requestID, version)
}
func (s *Service) ClaimFallback(ctx context.Context, p domain.Principal, eventID, requestID uuid.UUID) (store.AIFlashClaim, error) {
	return s.repository.ClaimAIEventFallback(ctx, p, eventID, requestID)
}
func (s *Service) Claim(ctx context.Context, p domain.Principal, id int64) (store.AIFlashClaim, error) {
	return s.repository.GetAIEventClaim(ctx, p, id)
}
func (s *Service) Reconcile(ctx context.Context) error {
	return s.repository.ReconcileAIEventClaimCounts(ctx)
}
func (s *Service) SetReady(ctx context.Context, id uuid.UUID, ready bool) error {
	return s.repository.SetAIEventReservationReady(ctx, id, ready)
}
func (s *Service) Reservation(ctx context.Context) (store.AIEventReservationState, error) {
	return s.repository.GetAIEventReservationState(ctx)
}
