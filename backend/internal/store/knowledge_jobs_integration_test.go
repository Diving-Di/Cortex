package store

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestKnowledgeJobStaleOwnerIsFenced(t *testing.T) {
	appURL, adminURL := os.Getenv("DATABASE_URL"), os.Getenv("MIGRATION_DATABASE_URL")
	if appURL == "" || adminURL == "" {
		t.Skip("database URLs are not configured")
	}
	ctx := context.Background()
	app, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	s := &Store{Pool: app, AdminPool: admin}
	job := KnowledgeIndexJob{ID: -1, TenantID: uuid.New(), DocumentID: uuid.New(), TargetVersion: 2, LeaseOwner: uuid.New()}
	if err := s.WriteKnowledgeChunks(ctx, job, nil, nil, "test"); !errors.Is(err, ErrKnowledgeIndexLeaseLost) {
		t.Fatalf("complete err=%v", err)
	}
	if err := s.FailKnowledgeJob(ctx, job, "TEST_FAILURE"); !errors.Is(err, ErrKnowledgeIndexLeaseLost) {
		t.Fatalf("fail err=%v", err)
	}
}
