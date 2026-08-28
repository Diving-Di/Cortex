package config

import (
	"path/filepath"
	"testing"
)

func TestLoadKnowledgeIndexDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://cortex_app:test@localhost/diary")
	t.Setenv("MIGRATION_DATABASE_URL", "postgresql://cortex_migrator:test@localhost/diary")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RerankModel != "BAAI/bge-reranker-v2-m3" {
		t.Fatalf("RerankModel = %q", cfg.RerankModel)
	}
	if cfg.KnowledgeIndexBatchSize != 16 || cfg.KnowledgeIndexPollSeconds != 5 {
		t.Fatalf("knowledge index defaults = %d/%d", cfg.KnowledgeIndexBatchSize, cfg.KnowledgeIndexPollSeconds)
	}
	if cfg.RedisURL != "redis://redis:6379/0" {
		t.Fatalf("RedisURL = %q", cfg.RedisURL)
	}
	if cfg.AIEventClaimIPLimit != 60 {
		t.Fatalf("AIEventClaimIPLimit = %d", cfg.AIEventClaimIPLimit)
	}
	if cfg.AuthPoolSize != 32 || cfg.AIEventClaimConcurrency != 16 || cfg.AIEventClaimQueueTimeout != 10_000_000_000 {
		t.Fatalf("AI event capacity defaults = auth pool %d, concurrency %d, timeout %s", cfg.AuthPoolSize, cfg.AIEventClaimConcurrency, cfg.AIEventClaimQueueTimeout)
	}
	if cfg.RuntimeRole != "all" {
		t.Fatalf("RuntimeRole = %q", cfg.RuntimeRole)
	}
}

func TestLoadRejectsInvalidRuntimeRole(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://cortex_app:test@localhost/cortex")
	t.Setenv("CORTEX_RUNTIME_ROLE", "scheduler-api")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid runtime role error")
	}
}

func TestLoadAIEventClaimIPLimit(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://cortex_app:test@localhost/diary")
	t.Setenv("MIGRATION_DATABASE_URL", "postgresql://cortex_migrator:test@localhost/diary")
	t.Setenv("AI_EVENT_CLAIM_IP_LIMIT", "1200")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AIEventClaimIPLimit != 1200 {
		t.Fatalf("AIEventClaimIPLimit = %d", cfg.AIEventClaimIPLimit)
	}
}

func TestLoadAIEventCapacitySettings(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://cortex_app:test@localhost/diary")
	t.Setenv("MIGRATION_DATABASE_URL", "postgresql://cortex_migrator:test@localhost/diary")
	t.Setenv("AUTH_DB_POOL_SIZE", "48")
	t.Setenv("AI_EVENT_CLAIM_CONCURRENCY", "24")
	t.Setenv("AI_EVENT_CLAIM_QUEUE_TIMEOUT_MS", "2500")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuthPoolSize != 48 || cfg.AIEventClaimConcurrency != 24 || cfg.AIEventClaimQueueTimeout.Milliseconds() != 2500 {
		t.Fatalf("AI event capacity settings = auth pool %d, concurrency %d, timeout %s", cfg.AuthPoolSize, cfg.AIEventClaimConcurrency, cfg.AIEventClaimQueueTimeout)
	}
}

func TestCortexDataDirTakesPrecedenceOverLegacyAlias(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://cortex_app:test@localhost/diary")
	t.Setenv("MIGRATION_DATABASE_URL", "postgresql://cortex_migrator:test@localhost/diary")
	t.Setenv("CORTEX_DATA_DIR", "./cortex-data")
	t.Setenv("DIARY_DATA_DIR", "./legacy-data")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs("./cortex-data")
	if cfg.DataDir != want {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, want)
	}
}

func TestLegacyDataDirRemainsCompatible(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://cortex_app:test@localhost/diary")
	t.Setenv("MIGRATION_DATABASE_URL", "postgresql://cortex_migrator:test@localhost/diary")
	t.Setenv("CORTEX_DATA_DIR", "")
	t.Setenv("DIARY_DATA_DIR", "./legacy-data")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs("./legacy-data")
	if cfg.DataDir != want {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, want)
	}
}

func TestExternalInfrastructureFailFast(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://cortex_app:test@localhost/cortex")
	t.Setenv("MIGRATION_DATABASE_URL", "postgresql://cortex_migrator:test@localhost/cortex")
	t.Setenv("STORAGE_BACKEND", "minio")
	if _, err := Load(); err == nil {
		t.Fatal("missing MinIO credentials accepted")
	}
	t.Setenv("MINIO_ENDPOINT", "http://minio:9000")
	t.Setenv("MINIO_ACCESS_KEY", "app")
	t.Setenv("MINIO_SECRET_KEY", "secret")
	t.Setenv("EVENT_BUS", "kafka")
	if _, err := Load(); err == nil {
		t.Fatal("missing Kafka URL accepted")
	}
	t.Setenv("KAFKA_REST_URL", "http://kafka:8082")
	t.Setenv("RAG_RETRIEVAL_BACKEND", "elasticsearch")
	if _, err := Load(); err == nil {
		t.Fatal("missing Elasticsearch URL accepted")
	}
	t.Setenv("ELASTICSEARCH_URLS", "http://elasticsearch:9200")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}
