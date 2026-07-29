package config

import (
	"strings"
	"testing"
)

func TestLoadValidatesKnowledgeChunkBoundaries(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://diary_app:test@localhost/diary")
	t.Setenv("MIGRATION_DATABASE_URL", "postgresql://diary_migrator:test@localhost/diary")

	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{"overlap", "RAG_CHILD_OVERLAP_TOKENS", "350", "must be smaller"},
		{"child max", "RAG_CHILD_MAX_TOKENS", "2500", "must be smaller"},
		{"parent target", "RAG_PARENT_TARGET_TOKENS", "2501", "must not exceed"},
		{"negative overlap", "RAG_CHILD_OVERLAP_TOKENS", "-1", "non-negative"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(test.key, test.value)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadKnowledgeChunkDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://diary_app:test@localhost/diary")
	t.Setenv("MIGRATION_DATABASE_URL", "postgresql://diary_migrator:test@localhost/diary")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.KnowledgeParentTarget != 1800 || cfg.KnowledgeParentMax != 2500 ||
		cfg.KnowledgeChildTarget != 350 || cfg.KnowledgeChildMax != 500 ||
		cfg.KnowledgeChildOverlap != 50 {
		t.Fatalf("unexpected chunk defaults: %#v", cfg)
	}
	if cfg.RerankModel != "Qwen/Qwen3-Reranker-0.6B" {
		t.Fatalf("RerankModel = %q", cfg.RerankModel)
	}
	if cfg.AIRuntime != "legacy" {
		t.Fatalf("AIRuntime = %q", cfg.AIRuntime)
	}
	if len(cfg.AIEinoWorkflows) != 0 {
		t.Fatalf("AIEinoWorkflows = %#v", cfg.AIEinoWorkflows)
	}
}

func TestLoadValidatesEinoWorkflows(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://diary_app:test@localhost/diary")
	t.Setenv("MIGRATION_DATABASE_URL", "postgresql://diary_migrator:test@localhost/diary")
	t.Setenv("AI_EINO_WORKFLOWS", "organize,report,knowledge")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cfg.AIEinoWorkflows, ",") != "organize,report,knowledge" {
		t.Fatalf("AIEinoWorkflows = %#v", cfg.AIEinoWorkflows)
	}
	t.Setenv("AI_EINO_WORKFLOWS", "organize,direct-vendor")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "AI_EINO_WORKFLOWS") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadValidatesAIRuntime(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://diary_app:test@localhost/diary")
	t.Setenv("MIGRATION_DATABASE_URL", "postgresql://diary_migrator:test@localhost/diary")
	t.Setenv("AI_RUNTIME", "direct-vendor")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "AI_RUNTIME") {
		t.Fatalf("Load() error = %v", err)
	}
}
