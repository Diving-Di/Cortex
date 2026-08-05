package config

import (
	"path/filepath"
	"testing"
)

func TestLoadKnowledgeIndexDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://diary_app:test@localhost/diary")
	t.Setenv("MIGRATION_DATABASE_URL", "postgresql://diary_migrator:test@localhost/diary")
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
}

func TestCortexDataDirTakesPrecedenceOverLegacyAlias(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://diary_app:test@localhost/diary")
	t.Setenv("MIGRATION_DATABASE_URL", "postgresql://diary_migrator:test@localhost/diary")
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
	t.Setenv("DATABASE_URL", "postgresql://diary_app:test@localhost/diary")
	t.Setenv("MIGRATION_DATABASE_URL", "postgresql://diary_migrator:test@localhost/diary")
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
