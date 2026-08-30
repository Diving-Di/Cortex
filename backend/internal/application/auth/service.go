package auth

import (
	"context"
	"net/mail"
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

var commonPasswords = map[string]struct{}{
	"123456789012": {}, "password1234": {}, "qwerty123456": {},
	"admin12345678": {}, "cortex123456": {},
}

func (e *ValidationError) Error() string { return e.Message }

func NewService(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) Register(ctx context.Context, username, email, password string) error {
	username, email = strings.TrimSpace(username), strings.ToLower(strings.TrimSpace(email))
	if len([]rune(username)) < 6 || len([]rune(username)) > 64 {
		return &ValidationError{Message: "用户名长度需为 6 到 64 个字符"}
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 {
		return &ValidationError{Message: "邮箱格式无效"}
	}
	if len([]rune(password)) < 12 {
		return &ValidationError{Message: "密码长度需至少 12 个字符"}
	}
	if len([]rune(password)) > 128 {
		return &ValidationError{Message: "密码长度不能超过 128 个字符"}
	}
	if strings.EqualFold(password, username) || isCommonPassword(password) {
		return &ValidationError{Message: "密码过于常见，请使用更强的密码"}
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	return s.repository.Register(ctx, username, email, hash)
}

func isCommonPassword(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	_, found := commonPasswords[value]
	return found
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
