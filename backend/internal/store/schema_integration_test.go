package store

import (
	"context"
	"os"
	"testing"

	"cortex/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProductionSchemaAndRLSContract(t *testing.T) {
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

	var version, tableCount, businessTableCount int
	if err := admin.QueryRow(ctx, `SELECT max(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 41 {
		t.Fatalf("migration version = %d, want 41", version)
	}
	if err := admin.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE tablename <> 'schema_migrations')
		FROM pg_tables WHERE schemaname='public'`).Scan(&tableCount, &businessTableCount); err != nil {
		t.Fatal(err)
	}
	if tableCount != 57 || businessTableCount != 56 {
		t.Fatalf("public table count = %d (%d business), want 57 (56 business)", tableCount, businessTableCount)
	}
	var unsafeTables []string
	rows, err := admin.Query(ctx, `
		SELECT c.relname
		FROM pg_class c
		JOIN pg_namespace n ON n.oid=c.relnamespace
		JOIN pg_attribute a ON a.attrelid=c.oid AND a.attname='tenant_id' AND NOT a.attisdropped
		WHERE n.nspname='public' AND c.relkind='r' AND (NOT c.relrowsecurity OR NOT c.relforcerowsecurity)
		ORDER BY c.relname`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		unsafeTables = append(unsafeTables, name)
	}
	rows.Close()
	if len(unsafeTables) != 0 {
		t.Fatalf("tenant tables without FORCE RLS: %v", unsafeTables)
	}

	var currentUser string
	var canCreate bool
	if err := app.QueryRow(ctx, `SELECT current_user, has_schema_privilege(current_user,'public','CREATE')`).Scan(&currentUser, &canCreate); err != nil {
		t.Fatal(err)
	}
	if currentUser != "cortex_app" || canCreate {
		t.Fatalf("application role = %s create_schema=%t", currentUser, canCreate)
	}

	assertCrossTenantNoteIsHidden(t, ctx, app, admin)
}

func assertCrossTenantNoteIsHidden(t *testing.T, ctx context.Context, app, admin *pgxpool.Pool) {
	t.Helper()
	username := "rls_" + uuid.NewString()
	var userA, userB, noteID int32
	if err := admin.QueryRow(ctx, `INSERT INTO users(username,email,password_hash) VALUES($1,$2,'test') RETURNING id`, username+"a", username+"a@example.invalid").Scan(&userA); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, `DELETE FROM users WHERE id=$1`, userA)
	if err := admin.QueryRow(ctx, `INSERT INTO users(username,email,password_hash) VALUES($1,$2,'test') RETURNING id`, username+"b", username+"b@example.invalid").Scan(&userB); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, `DELETE FROM users WHERE id=$1`, userB)
	tenantA, tenantB := uuid.New(), uuid.New()
	if _, err := admin.Exec(ctx, `INSERT INTO tenants(id,user_id,name) VALUES($1,$2,'A'),($3,$4,'B')`, tenantA, userA, tenantB, userB); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, `INSERT INTO notes(tenant_id,created_by,updated_by,title) VALUES($1,$2,$2,'private') RETURNING id`, tenantA, userA).Scan(&noteID); err != nil {
		t.Fatal(err)
	}
	store := &Store{Pool: app}
	_, err := store.GetNote(ctx, domain.Principal{TenantID: tenantB, UserID: userB, TenantActive: true}, noteID)
	// GetNote deliberately maps pgx.ErrNoRows to the public 404 error. The
	// assertion only requires that tenant B cannot retrieve tenant A's row.
	if err == nil {
		t.Fatal("cross-tenant note was visible")
	}
}
