package config

import "testing"

func TestLoadRecipeRetrievalDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://diary_app:test@localhost/diary")
	t.Setenv("MIGRATION_DATABASE_URL", "postgresql://diary_migrator:test@localhost/diary")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RerankModel != "BAAI/bge-reranker-v2-m3" {
		t.Fatalf("RerankModel = %q", cfg.RerankModel)
	}
}
