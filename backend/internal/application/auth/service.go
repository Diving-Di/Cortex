package auth

import (
	"context"
	"strings"
	"time"

	"cortex/backend/internal/auth"
	"cortex/backend/internal/domain"
)

type Repository interface {
	Register(context.Context, string, string, string) error
	Login(context.Context, string, string, time.Duration) (string, string, error)
	RevokeToken(context.Context, int32) error
	ResolvePrincipal(context.Context, string) (domain.Principal, error)
}

type Service struct{ repository Repository }

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

func NewService(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) Register(ctx context.Context, username, email, password string) error {
	username, email = strings.TrimSpace(username), strings.TrimSpace(email)
	if len(username) < 6 {
		return &ValidationError{Message: "用户名长度需至少 6 个字符"}
	}
	if len(password) < 6 {
		return &ValidationError{Message: "密码长度需至少 6 个字符"}
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	return s.repository.Register(ctx, username, email, hash)
}
func (s *Service) Login(ctx context.Context, username, password string, ttl time.Duration) (string, string, error) {
	return s.repository.Login(ctx, strings.TrimSpace(username), password, ttl)
}
func (s *Service) Revoke(ctx context.Context, tokenID int32) error {
	return s.repository.RevokeToken(ctx, tokenID)
}
func (s *Service) Resolve(ctx context.Context, token string) (domain.Principal, error) {
	return s.repository.ResolvePrincipal(ctx, token)
}
