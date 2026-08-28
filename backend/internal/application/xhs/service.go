package xhs

import (
	"context"
	"time"

	"cortex/backend/internal/domain"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

type Repository interface {
	GetXHSAuthorization(context.Context, domain.Principal) (store.XHSAuthorization, error)
	CreateXHSAuthAttempt(context.Context, domain.Principal, time.Time, int) (store.XHSAuthAttempt, error)
	GetXHSAuthAttempt(context.Context, domain.Principal, uuid.UUID) (store.XHSAuthAttempt, error)
	CancelXHSAuthAttempt(context.Context, domain.Principal, uuid.UUID) error
	RevokeXHSAuthorization(context.Context, domain.Principal) error
	MarkXHSAuthorizationVerified(context.Context, domain.Principal, bool) error
	ClaimXHSAuthAttempts(context.Context, string, int, time.Duration) ([]store.XHSAuthAttempt, error)
	UpdateXHSAuthAttempt(context.Context, domain.Principal, uuid.UUID, string, *string, string) error
	CompleteXHSAuthorization(context.Context, domain.Principal, uuid.UUID, []byte, []byte, int, string, *time.Time) error
	FailXHSAuthAttempt(context.Context, domain.Principal, uuid.UUID, string) error
	AcquireXHSAuthorizationLease(context.Context, domain.Principal, string, time.Duration) (bool, error)
	ReleaseXHSAuthorizationLease(context.Context, domain.Principal, string) error
}
type Service struct{ repository Repository }

func NewService(r Repository) *Service { return &Service{repository: r} }
func (s *Service) Authorization(c context.Context, p domain.Principal) (store.XHSAuthorization, error) {
	return s.repository.GetXHSAuthorization(c, p)
}
func (s *Service) Start(c context.Context, p domain.Principal, e time.Time, k int) (store.XHSAuthAttempt, error) {
	return s.repository.CreateXHSAuthAttempt(c, p, e, k)
}
func (s *Service) Attempt(c context.Context, p domain.Principal, id uuid.UUID) (store.XHSAuthAttempt, error) {
	return s.repository.GetXHSAuthAttempt(c, p, id)
}
func (s *Service) GetXHSAuthAttempt(c context.Context, p domain.Principal, id uuid.UUID) (store.XHSAuthAttempt, error) {
	return s.repository.GetXHSAuthAttempt(c, p, id)
}
func (s *Service) Cancel(c context.Context, p domain.Principal, id uuid.UUID) error {
	return s.repository.CancelXHSAuthAttempt(c, p, id)
}
func (s *Service) Revoke(c context.Context, p domain.Principal) error {
	return s.repository.RevokeXHSAuthorization(c, p)
}
func (s *Service) Verify(c context.Context, p domain.Principal, v bool) error {
	return s.repository.MarkXHSAuthorizationVerified(c, p, v)
}
func (s *Service) MarkXHSAuthorizationVerified(c context.Context, p domain.Principal, v bool) error {
	return s.repository.MarkXHSAuthorizationVerified(c, p, v)
}
func (s *Service) ClaimXHSAuthAttempts(c context.Context, o string, n int, d time.Duration) ([]store.XHSAuthAttempt, error) {
	return s.repository.ClaimXHSAuthAttempts(c, o, n, d)
}
func (s *Service) UpdateXHSAuthAttempt(c context.Context, p domain.Principal, id uuid.UUID, status string, qr *string, code string) error {
	return s.repository.UpdateXHSAuthAttempt(c, p, id, status, qr, code)
}
func (s *Service) CompleteXHSAuthorization(c context.Context, p domain.Principal, id uuid.UUID, cipher, nonce []byte, version int, name string, expires *time.Time) error {
	return s.repository.CompleteXHSAuthorization(c, p, id, cipher, nonce, version, name, expires)
}
func (s *Service) FailXHSAuthAttempt(c context.Context, p domain.Principal, id uuid.UUID, code string) error {
	return s.repository.FailXHSAuthAttempt(c, p, id, code)
}
func (s *Service) AcquireXHSAuthorizationLease(c context.Context, p domain.Principal, owner string, lease time.Duration) (bool, error) {
	return s.repository.AcquireXHSAuthorizationLease(c, p, owner, lease)
}
func (s *Service) ReleaseXHSAuthorizationLease(c context.Context, p domain.Principal, owner string) error {
	return s.repository.ReleaseXHSAuthorizationLease(c, p, owner)
}
