package auth

import (
	"context"
	"testing"
	"time"

	"cortex/backend/internal/domain"
)

type repositoryStub struct{ registered bool }

func (r *repositoryStub) Register(context.Context, string, string, string) error {
	r.registered = true
	return nil
}
func (*repositoryStub) Login(context.Context, string, string, time.Duration) (string, string, error) {
	return "", "", nil
}
func (*repositoryStub) RevokeToken(context.Context, int32) error { return nil }
func (*repositoryStub) ResolvePrincipal(context.Context, string) (domain.Principal, error) {
	return domain.Principal{}, nil
}

func TestRegisterEnforcesProductionPasswordAndEmailPolicy(t *testing.T) {
	tests := []struct {
		name, email, password string
	}{
		{"short password", "person@example.com", "short123"},
		{"common password", "person@example.com", "password1234"},
		{"invalid email", "not-an-email", "strong passphrase 2026"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &repositoryStub{}
			err := NewService(repository).Register(context.Background(), "tester01", test.email, test.password)
			if err == nil || repository.registered {
				t.Fatalf("weak registration accepted: err=%v registered=%t", err, repository.registered)
			}
		})
	}
}

func TestRegisterAcceptsStrongCredentials(t *testing.T) {
	repository := &repositoryStub{}
	if err := NewService(repository).Register(context.Background(), "tester01", "person@example.com", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if !repository.registered {
		t.Fatal("repository was not called")
	}
}
