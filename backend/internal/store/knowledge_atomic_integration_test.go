package store

import (
	"context"
	"errors"
	"os"
	"testing"

	"cortex/backend/internal/knowledge"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestKnowledgeChunksRollbackBeforeActivationKeepsOldVersion(t *testing.T) {
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
	username := "atomic_" + uuid.NewString()
	tenantID, documentID, parentID, owner := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	var userID int32
	var jobID int64
	if err := admin.QueryRow(ctx, `INSERT INTO users(username,email,password_hash) VALUES($1,$2,'test') RETURNING id`, username, username+"@example.invalid").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)
	if _, err := admin.Exec(ctx, `INSERT INTO tenants(id,user_id,name) VALUES($1,$2,'atomic integration')`, tenantID, userID); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, `DELETE FROM tenants WHERE id=$1`, tenantID)
	uploadID := uuid.New()
	if _, err := admin.Exec(ctx, `INSERT INTO knowledge_uploads(id,tenant_id,original_name,stored_root,status) VALUES($1,$2,'old.md',$3,'ready')`, uploadID, tenantID, "knowledge/"+tenantID.String()+"/fixture/source"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO knowledge_documents(id,tenant_id,upload_id,source_type,title,stored_path,content_hash,active_index_version,status,knowledge_enabled) VALUES($1,$2,$3,'upload','old',$4,'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',1,'ready',true)`, documentID, tenantID, uploadID, "knowledge/"+tenantID.String()+"/fixture/source/old.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO knowledge_parent_chunks(id,tenant_id,document_id,index_version,ordinal,content,content_hash) VALUES($1,$2,$3,1,0,'old content','bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb')`, parentID, tenantID, documentID); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, `INSERT INTO knowledge_index_jobs(tenant_id,document_id,target_index_version,status,lease_owner,lease_until,attempts) VALUES($1,$2,2,'running',$3,now()+interval '5 minutes',1) RETURNING id`, tenantID, documentID, owner).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	parents := knowledge.Chunk("new", "upload", "# New\nnew content")
	vectors := make([][][]float32, len(parents))
	for i, p := range parents {
		vectors[i] = make([][]float32, len(p.Children))
		for j := range p.Children {
			vectors[i][j] = make([]float32, 512)
		}
	}
	injected := errors.New("crash before activation")
	job := KnowledgeIndexJob{ID: jobID, TenantID: tenantID, DocumentID: documentID, TargetVersion: 2, LeaseOwner: owner}
	if err := s.writeKnowledgeChunks(ctx, job, parents, vectors, "test", func() error { return injected }); !errors.Is(err, injected) {
		t.Fatalf("err=%v", err)
	}
	var active int
	var status string
	var newParents int
	if err := admin.QueryRow(ctx, `SELECT active_index_version,status FROM knowledge_documents WHERE id=$1`, documentID).Scan(&active, &status); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM knowledge_parent_chunks WHERE document_id=$1 AND index_version=2`, documentID).Scan(&newParents); err != nil {
		t.Fatal(err)
	}
	if active != 1 || status != "ready" || newParents != 0 {
		t.Fatalf("active=%d status=%s newParents=%d", active, status, newParents)
	}
	var jobStatus string
	if err := admin.QueryRow(ctx, `SELECT status FROM knowledge_index_jobs WHERE id=$1`, jobID).Scan(&jobStatus); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "running" {
		t.Fatalf("job status=%s", jobStatus)
	}
}
