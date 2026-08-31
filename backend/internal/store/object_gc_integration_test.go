package store

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"cortex/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestObjectGCReclaimsExpiredLeaseAndPurgesAttachment(t *testing.T) {
	_, admin := gcTestPools(t)
	ctx := context.Background()
	userID, tenantID, noteID := gcTestTenant(t, ctx, admin, 1024)
	defer admin.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)

	key, version := "tenants/"+tenantID.String()+"/attachments/deleted", "version-7"
	var attachmentID int32
	if err := admin.QueryRow(ctx, `INSERT INTO attachments(tenant_id,uploaded_by,note_id,original_name,stored_path,storage_backend,object_key,object_version,mime_type,size,sha256,deleted_at) VALUES($1,$2,$3,'deleted.txt',$4,'minio',$5,$6,'text/plain',10,$7,now()) RETURNING id`, tenantID, userID, noteID, key, key, version, strings.Repeat("0", 64)).Scan(&attachmentID); err != nil {
		t.Fatal(err)
	}
	oldOwner := uuid.New()
	var jobID int64
	if err := admin.QueryRow(ctx, `INSERT INTO object_gc_jobs(tenant_id,storage_backend,object_key,object_version,status,lease_owner,lease_expires_at) VALUES($1,'minio',$2,$3,'running',$4,now()-interval '1 minute') RETURNING id`, tenantID, key, version, oldOwner).Scan(&jobID); err != nil {
		t.Fatal(err)
	}

	s := &Store{AdminPool: admin}
	job, err := s.ClaimObjectGC(ctx, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if job == nil || job.ID != jobID || job.LeaseOwner == oldOwner || job.ObjectVersion != version {
		t.Fatalf("reclaimed job = %#v", job)
	}
	stale := *job
	stale.LeaseOwner = oldOwner
	if err = s.FinishObjectGC(ctx, stale, true); err != nil {
		t.Fatal(err)
	}
	var status string
	var owner uuid.UUID
	if err = admin.QueryRow(ctx, `SELECT status,lease_owner FROM object_gc_jobs WHERE id=$1`, jobID).Scan(&status, &owner); err != nil {
		t.Fatal(err)
	}
	if status != "running" || owner != job.LeaseOwner {
		t.Fatalf("stale worker changed active lease: status=%s owner=%s", status, owner)
	}
	if err = s.FinishObjectGC(ctx, *job, true); err != nil {
		t.Fatal(err)
	}
	var attachmentCount int
	if err = admin.QueryRow(ctx, `SELECT count(*) FROM attachments WHERE id=$1`, attachmentID).Scan(&attachmentCount); err != nil {
		t.Fatal(err)
	}
	if attachmentCount != 0 {
		t.Fatal("soft-deleted attachment row was not purged after object deletion")
	}
}

func TestAttachmentQuotaExcludesSoftDeletedRows(t *testing.T) {
	app, admin := gcTestPools(t)
	ctx := context.Background()
	userID, tenantID, noteID := gcTestTenant(t, ctx, admin, 10)
	defer admin.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)
	oldPath := "tenants/" + tenantID.String() + "/quota-old"
	newPath := "tenants/" + tenantID.String() + "/quota-new"
	if _, err := admin.Exec(ctx, `INSERT INTO attachments(tenant_id,uploaded_by,note_id,original_name,stored_path,mime_type,size,sha256,deleted_at) VALUES($1,$2,$3,'old.txt',$4,'text/plain',10,$5,now())`, tenantID, userID, noteID, oldPath, strings.Repeat("0", 64)); err != nil {
		t.Fatal(err)
	}
	s := &Store{Pool: app}
	_, err := s.AddAttachment(ctx, domain.Principal{TenantID: tenantID, UserID: userID, TenantActive: true}, domain.Attachment{NoteID: noteID, OriginalName: "new.txt", StoredPath: newPath, StorageBackend: "local", MIMEType: "text/plain", Size: 10, SHA256: strings.Repeat("0", 64)})
	if err != nil {
		t.Fatalf("soft-deleted attachment still consumed quota: %v", err)
	}
}

func gcTestPools(t *testing.T) (*pgxpool.Pool, *pgxpool.Pool) {
	t.Helper()
	appURL, adminURL := os.Getenv("DATABASE_URL"), os.Getenv("MIGRATION_DATABASE_URL")
	if appURL == "" || adminURL == "" {
		t.Skip("database URLs are not configured")
	}
	ctx := context.Background()
	app, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		app.Close()
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	t.Cleanup(admin.Close)
	return app, admin
}

func gcTestTenant(t *testing.T, ctx context.Context, admin *pgxpool.Pool, quota int64) (int32, uuid.UUID, int32) {
	t.Helper()
	name := "gc_" + uuid.NewString()
	var userID, noteID int32
	if err := admin.QueryRow(ctx, `INSERT INTO users(username,email,password_hash) VALUES($1,$2,'test') RETURNING id`, name, name+"@example.invalid").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	tenantID := uuid.New()
	if _, err := admin.Exec(ctx, `INSERT INTO tenants(id,user_id,name,attachment_quota_bytes) VALUES($1,$2,'GC test',$3)`, tenantID, userID, quota); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, `INSERT INTO notes(tenant_id,created_by,updated_by,title) VALUES($1,$2,$2,'GC test') RETURNING id`, tenantID, userID).Scan(&noteID); err != nil {
		t.Fatal(err)
	}
	return userID, tenantID, noteID
}
