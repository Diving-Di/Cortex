package config

import (
	"path/filepath"
	"strings"
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
	if cfg.AuthLoginIPLimit != 30 || cfg.AuthLoginAccountLimit != 10 || cfg.AuthRegisterIPLimit != 5 || cfg.AuthTokenIPLimit != 15 {
		t.Fatalf("auth rate defaults = %d/%d/%d/%d", cfg.AuthLoginIPLimit, cfg.AuthLoginAccountLimit, cfg.AuthRegisterIPLimit, cfg.AuthTokenIPLimit)
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

func TestProductionConfigurationFailsClosed(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://cortex_app:test@localhost/cortex")
	t.Setenv("MIGRATION_DATABASE_URL", "postgresql://cortex_migrator:test@localhost/cortex")
	t.Setenv("APP_ENV", "production")
	t.Setenv("REDIS_URL", "redis://default:strong-secret@redis:6379/0")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "PUBLIC_BASE_URL") {
		t.Fatalf("missing public URL error = %v", err)
	}
	t.Setenv("PUBLIC_BASE_URL", "http://cortex.example.com")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "PUBLIC_BASE_URL") {
		t.Fatalf("insecure public URL error = %v", err)
	}
	t.Setenv("PUBLIC_BASE_URL", "https://cortex.example.com")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "CORS_ORIGINS") {
		t.Fatalf("missing explicit CORS error = %v", err)
	}
	t.Setenv("CORS_ORIGINS", "http://cortex.example.com")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "https origins") {
		t.Fatalf("insecure CORS error = %v", err)
	}
	t.Setenv("CORS_ORIGINS", "https://cortex.example.com")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicBaseURL != "https://cortex.example.com" || cfg.Environment != "production" {
		t.Fatalf("production config = %#v", cfg)
	}
}

func TestProductionRejectsDefaultRedisPassword(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://cortex_app:test@localhost/cortex")
	t.Setenv("MIGRATION_DATABASE_URL", "postgresql://cortex_migrator:test@localhost/cortex")
	t.Setenv("APP_ENV", "production")
	t.Setenv("PUBLIC_BASE_URL", "https://cortex.example.com")
	t.Setenv("CORS_ORIGINS", "https://cortex.example.com")
	t.Setenv("REDIS_URL", "redis://default:change-me@redis:6379/0")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "REDIS_URL") {
		t.Fatalf("default Redis password error = %v", err)
	}
}
