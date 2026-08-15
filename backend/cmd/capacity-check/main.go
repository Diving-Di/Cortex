package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"cortex/backend/internal/config"
	"cortex/backend/internal/domain"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type stageResult struct {
	Documents           int     `json:"documents"`
	Queries             int     `json:"queries"`
	Failures            int     `json:"failures"`
	LoadSeconds         float64 `json:"load_seconds"`
	DocumentsPerSecond  float64 `json:"documents_per_second"`
	RetrievalP50MS      float64 `json:"retrieval_p50_ms"`
	RetrievalP95MS      float64 `json:"retrieval_p95_ms"`
	RetrievalP99MS      float64 `json:"retrieval_p99_ms"`
	RelationsBytes      int64   `json:"relations_bytes"`
	RelationGrowthBytes int64   `json:"relation_growth_bytes"`
	SyntheticBytes      int64   `json:"synthetic_source_bytes"`
}

type report struct {
	GeneratedAtUTC string        `json:"generated_at_utc"`
	GitCommit      string        `json:"git_commit"`
	GitDirty       bool          `json:"git_dirty"`
	Model          string        `json:"embedding_model"`
	Dimensions     int           `json:"embedding_dimensions"`
	Stages         []stageResult `json:"stages"`
	AICost         string        `json:"ai_cost"`
	Cleanup        string        `json:"cleanup"`
}

func main() {
	sizesArg := flag.String("sizes", "100,1000,10000", "cumulative synthetic document counts")
	queries := flag.Int("queries", 100, "retrieval queries per stage")
	flag.Parse()
	cfg, err := config.Load()
	must(err)
	ctx := context.Background()
	db, err := store.Open(ctx, cfg)
	must(err)
	defer db.Close()
	sizes := parseSizes(*sizesArg)
	if len(sizes) == 0 || *queries <= 0 {
		must(fmt.Errorf("sizes and queries must be positive"))
	}
	principal, cleanup := createSyntheticTenant(ctx, db)
	defer cleanup()
	baseBytes := relationBytes(ctx, db)
	result := report{GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339Nano), GitCommit: os.Getenv("CAPACITY_GIT_COMMIT"), GitDirty: os.Getenv("CAPACITY_GIT_DIRTY") == "true", Model: "capacity-synthetic-v1", Dimensions: 512, AICost: "not_measured_no_ai_calls", Cleanup: "synthetic_tenant_cascade_delete"}
	loaded := 0
	for _, target := range sizes {
		started := time.Now()
		insertSynthetic(ctx, db, principal, loaded, target, result.Model)
		loadSeconds := time.Since(started).Seconds()
		loaded = target
		latencies := make([]float64, 0, *queries)
		failures := 0
		vector := make([]float32, 512)
		vector[0] = 1
		for i := 0; i < *queries; i++ {
			query := fmt.Sprintf("capacitytoken%d", i%target)
			started := time.Now()
			items, searchErr := db.SearchKnowledge(ctx, principal, query, vector, result.Model, nil, 15, 10, 5, 20)
			latencies = append(latencies, float64(time.Since(started).Microseconds())/1000)
			if searchErr != nil || len(items) == 0 {
				failures++
			}
		}
		sort.Float64s(latencies)
		relations := relationBytes(ctx, db)
		result.Stages = append(result.Stages, stageResult{Documents: target, Queries: *queries, Failures: failures, LoadSeconds: round(loadSeconds), DocumentsPerSecond: round(float64(target-lenBefore(target, sizes)) / loadSeconds), RetrievalP50MS: percentile(latencies, .50), RetrievalP95MS: percentile(latencies, .95), RetrievalP99MS: percentile(latencies, .99), RelationsBytes: relations, RelationGrowthBytes: relations - baseBytes, SyntheticBytes: syntheticBytes(ctx, db, principal)})
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	must(err)
	fmt.Println(string(encoded))
}

func parseSizes(raw string) []int {
	var out []int
	last := 0
	for _, part := range strings.Split(raw, ",") {
		var value int
		if _, err := fmt.Sscanf(strings.TrimSpace(part), "%d", &value); err != nil || value <= last {
			return nil
		}
		out = append(out, value)
		last = value
	}
	return out
}

func createSyntheticTenant(ctx context.Context, db *store.Store) (domain.Principal, func()) {
	name := "capacity_" + uuid.NewString()
	var p domain.Principal
	p.TenantID = uuid.New()
	must(db.AdminPool.QueryRow(ctx, `INSERT INTO users(username,email,password_hash) VALUES($1,$2,'synthetic-not-loginable') RETURNING id`, name, name+"@example.invalid").Scan(&p.UserID))
	_, err := db.AdminPool.Exec(ctx, `INSERT INTO tenants(id,user_id,name,note_quota) VALUES($1,$2,'synthetic capacity',20000)`, p.TenantID, p.UserID)
	must(err)
	p.Username, p.TenantActive = name, true
	return p, func() { _, _ = db.AdminPool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, p.UserID) }
}

func insertSynthetic(ctx context.Context, db *store.Store, p domain.Principal, from, to int, model string) {
	if from >= to {
		return
	}
	tx, err := db.AdminPool.BeginTx(ctx, pgx.TxOptions{})
	must(err)
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `WITH fixture AS MATERIALIZED (
		SELECT i,gen_random_uuid() upload_id,gen_random_uuid() document_id
		FROM generate_series($2::int,$3::int) i
	), uploads AS (
		INSERT INTO knowledge_uploads(id,tenant_id,original_name,stored_root,expanded_bytes,status)
		SELECT upload_id,$1,'synthetic-'||i||'.md','capacity/'||upload_id,256,'ready' FROM fixture
	)
	INSERT INTO knowledge_documents(id,tenant_id,upload_id,source_type,title,stored_path,size_bytes,content_hash,active_index_version,status)
	SELECT document_id,$1,upload_id,'upload','synthetic capacity '||i,'capacity/'||upload_id||'/synthetic.md',256,
		encode(digest('capacity-document-'||i,'sha256'),'hex'),1,'ready' FROM fixture`, p.TenantID, from+1, to)
	must(err)
	_, err = tx.Exec(ctx, `INSERT INTO knowledge_parent_chunks(tenant_id,document_id,index_version,ordinal,heading_path,content,content_hash)
		SELECT $1,d.id,1,0,ARRAY['synthetic'],
		'synthetic capacity corpus capacitytoken'||substring(d.title from '[0-9]+$')||' repeated deterministic content',
		encode(digest('capacity-parent-'||d.id,'sha256'),'hex')
		FROM knowledge_documents d WHERE d.tenant_id=$1 AND substring(d.title from '[0-9]+$')::int BETWEEN $2 AND $3`, p.TenantID, from+1, to)
	must(err)
	vector := "[1," + strings.TrimSuffix(strings.Repeat("0,", 511), ",") + "]"
	_, err = tx.Exec(ctx, `INSERT INTO knowledge_child_chunks(tenant_id,parent_id,document_id,index_version,ordinal,content,embedding_text,embedding,embedding_model,content_hash,keyword_text)
		SELECT $1,p.id,p.document_id,1,0,p.content,p.content,$4::vector,$5,
		encode(digest('capacity-child-'||p.id,'sha256'),'hex'),
		'synthetic capacity corpus capacitytoken'||substring(d.title from '[0-9]+$')
		FROM knowledge_parent_chunks p JOIN knowledge_documents d ON d.tenant_id=p.tenant_id AND d.id=p.document_id
		WHERE p.tenant_id=$1 AND substring(d.title from '[0-9]+$')::int BETWEEN $2 AND $3`, p.TenantID, from+1, to, vector, model)
	must(err)
	_, err = tx.Exec(ctx, `INSERT INTO knowledge_quotas(tenant_id,used_bytes) VALUES($1,$2) ON CONFLICT(tenant_id) DO UPDATE SET used_bytes=excluded.used_bytes`, p.TenantID, int64(to)*256)
	must(err)
	must(tx.Commit(ctx))
}

func relationBytes(ctx context.Context, db *store.Store) int64 {
	var bytes int64
	must(db.AdminPool.QueryRow(ctx, `SELECT sum(pg_total_relation_size(c.oid)) FROM pg_class c WHERE c.relname IN ('knowledge_documents','knowledge_parent_chunks','knowledge_child_chunks')`).Scan(&bytes))
	return bytes
}

func syntheticBytes(ctx context.Context, db *store.Store, p domain.Principal) int64 {
	var bytes int64
	must(db.AdminPool.QueryRow(ctx, `SELECT COALESCE(sum(size_bytes),0) FROM knowledge_documents WHERE tenant_id=$1`, p.TenantID).Scan(&bytes))
	return bytes
}

func percentile(values []float64, p float64) float64 {
	index := int(math.Ceil(float64(len(values))*p)) - 1
	if index < 0 {
		return 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return round(values[index])
}

func round(value float64) float64 { return math.Round(value*1000) / 1000 }

func lenBefore(target int, sizes []int) int {
	previous := 0
	for _, size := range sizes {
		if size == target {
			return previous
		}
		previous = size
	}
	return 0
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var _ = pgx.CopyFromRows
