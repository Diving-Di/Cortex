package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL                  string
	MigrationDatabaseURL         string
	ListenAddress                string
	CORSOrigins                  []string
	TokenTTL                     time.Duration
	StatementTimeout             time.Duration
	PoolSize                     int32
	LogLevel                     slog.Level
	DataDir                      string
	MaxAttachmentBytes           int64
	EmbeddingBaseURL             string
	EmbeddingAPIKey              string
	EmbeddingModel               string
	EmbeddingDimensions          int
	EmbeddingSendDimensions      bool
	RerankBaseURL                string
	RerankModel                  string
	RAGVectorTopK                int
	RAGTitleTopK                 int
	RAGKeywordTopK               int
	RAGFusionTopK                int
	RAGContextTopK               int
	RAGRerankMinScore            *float64
	RAGRerankMinMargin           *float64
	RAGMinQualifiedEvidence      int
	RAGVerifierModel             string
	AIAPIKey                     string
	AIBaseURL                    string
	AIModel                      string
	AISystemPrompt               string
	Environment                  string
	ScheduledReportsEnabled      bool
	ScheduledReportPoll          time.Duration
	ResearchEnabled              bool
	ResearchWorkers              int
	ResearchMaxKeywords          int
	ResearchMaxURLs              int
	ResearchMaxResults           int
	ResearchMaxImages            int
	ResearchMaxImageBytes        int64
	ResearchMaxBodyChars         int
	ResearchLease                time.Duration
	ResearchMaxAttempts          int
	ResearchRequestInterval      time.Duration
	ResearchHTTPTimeout          time.Duration
	ResearchOCRURL               string
	XHSSessionEncryptionKey      string
	XHSSessionKeyVersion         int
	XHSAuthorizationTTL          time.Duration
	XHSAuthorizationEnabled      bool
	XHSChromePath                string
	KnowledgeIndexBatchSize      int
	KnowledgeIndexPollSeconds    int
	KnowledgeMaxUploadBytes      int64
	KnowledgeMaxExtractedBytes   int64
	KnowledgeMaxFileBytes        int64
	KnowledgeMaxFiles            int
	KnowledgeMaxDepth            int
	KnowledgeMaxCompressionRatio int
	RedisURL                     string
}

func Load() (Config, error) {
	databaseURL := normalizeDatabaseURL(strings.TrimSpace(os.Getenv("DATABASE_URL")))
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL must use PostgreSQL")
	}
	migrationDatabaseURL := normalizeDatabaseURL(valueOrDefault("MIGRATION_DATABASE_URL", databaseURL))
	if migrationDatabaseURL == "" {
		return Config{}, fmt.Errorf("MIGRATION_DATABASE_URL must use PostgreSQL")
	}
	origins := splitCSV(valueOrDefault("CORS_ORIGINS", "http://127.0.0.1:5173,http://localhost:5173"))
	if len(origins) == 0 {
		return Config{}, fmt.Errorf("CORS_ORIGINS must contain explicit trusted origins")
	}
	for _, origin := range origins {
		if origin == "*" {
			return Config{}, fmt.Errorf("CORS_ORIGINS must not contain *")
		}
	}
	tokenHours, err := positiveInt("TOKEN_TTL_HOURS", 720)
	if err != nil {
		return Config{}, err
	}
	pollSeconds, err := positiveInt("SCHEDULED_REPORT_POLL_SECONDS", 60)
	if err != nil {
		return Config{}, err
	}
	statementMS, err := positiveInt("DB_STATEMENT_TIMEOUT_MS", 15000)
	if err != nil {
		return Config{}, err
	}
	poolSize, err := positiveInt("DB_POOL_SIZE", 5)
	if err != nil {
		return Config{}, err
	}
	maxAttachmentBytes, err := positiveInt("MAX_ATTACHMENT_BYTES", 20*1024*1024)
	if err != nil {
		return Config{}, err
	}
	embeddingDimensions, err := positiveInt("RAG_EMBEDDING_DIMENSIONS", 512)
	if err != nil {
		return Config{}, err
	}
	dataDirSetting := strings.TrimSpace(os.Getenv("CORTEX_DATA_DIR"))
	if dataDirSetting == "" {
		dataDirSetting = strings.TrimSpace(os.Getenv("DIARY_DATA_DIR"))
		if dataDirSetting != "" {
			slog.Warn("DIARY_DATA_DIR is deprecated; use CORTEX_DATA_DIR")
		}
	}
	if dataDirSetting == "" {
		dataDirSetting = "./data"
	}
	dataDir, err := filepath.Abs(dataDirSetting)
	if err != nil {
		return Config{}, fmt.Errorf("resolve CORTEX_DATA_DIR: %w", err)
	}
	researchWorkers, err := positiveInt("RESEARCH_WORKERS", 1)
	if err != nil {
		return Config{}, err
	}
	researchMaxKeywords, err := positiveInt("RESEARCH_MAX_KEYWORDS", 5)
	if err != nil {
		return Config{}, err
	}
	researchMaxURLs, err := positiveInt("RESEARCH_MAX_URLS", 30)
	if err != nil {
		return Config{}, err
	}
	researchMaxResults, err := positiveInt("RESEARCH_MAX_RESULTS", 50)
	if err != nil {
		return Config{}, err
	}
	researchMaxImages, err := positiveInt("RESEARCH_MAX_IMAGES", 20)
	if err != nil {
		return Config{}, err
	}
	researchMaxImageBytes, err := positiveInt("RESEARCH_MAX_IMAGE_BYTES", 10*1024*1024)
	if err != nil {
		return Config{}, err
	}
	researchMaxBodyChars, err := positiveInt("RESEARCH_MAX_BODY_CHARS", 100_000)
	if err != nil {
		return Config{}, err
	}
	researchLeaseSeconds, err := positiveInt("RESEARCH_LEASE_SECONDS", 300)
	if err != nil {
		return Config{}, err
	}
	researchMaxAttempts, err := positiveInt("RESEARCH_MAX_ATTEMPTS", 3)
	if err != nil {
		return Config{}, err
	}
	researchIntervalMS, err := positiveInt("RESEARCH_REQUEST_INTERVAL_MS", 1500)
	if err != nil {
		return Config{}, err
	}
	researchTimeoutSeconds, err := positiveInt("RESEARCH_HTTP_TIMEOUT_SECONDS", 20)
	if err != nil {
		return Config{}, err
	}
	xhsKeyVersion, err := positiveInt("XHS_SESSION_KEY_VERSION", 1)
	if err != nil {
		return Config{}, err
	}
	xhsAuthorizationTTLSeconds, err := positiveInt("XHS_AUTHORIZATION_TTL_SECONDS", 180)
	if err != nil {
		return Config{}, err
	}
	knowledgeIndexBatchSize, err := positiveInt("KNOWLEDGE_INDEX_BATCH_SIZE", 16)
	if err != nil {
		return Config{}, err
	}
	knowledgeIndexPollSeconds, err := positiveInt("KNOWLEDGE_INDEX_POLL_SECONDS", 5)
	if err != nil {
		return Config{}, err
	}
	knowledgeMaxUploadBytes, err := positiveInt("KNOWLEDGE_MAX_UPLOAD_BYTES", 256*1024*1024)
	if err != nil {
		return Config{}, err
	}
	knowledgeMaxExtractedBytes, err := positiveInt("KNOWLEDGE_MAX_EXTRACTED_BYTES", 1024*1024*1024)
	if err != nil {
		return Config{}, err
	}
	knowledgeMaxFileBytes, err := positiveInt("KNOWLEDGE_MAX_FILE_BYTES", 64*1024*1024)
	if err != nil {
		return Config{}, err
	}
	knowledgeMaxFiles, err := positiveInt("KNOWLEDGE_MAX_FILES", 5000)
	if err != nil {
		return Config{}, err
	}
	knowledgeMaxDepth, err := positiveInt("KNOWLEDGE_MAX_DEPTH", 16)
	if err != nil {
		return Config{}, err
	}
	knowledgeMaxCompressionRatio, err := positiveInt("KNOWLEDGE_MAX_COMPRESSION_RATIO", 100)
	if err != nil {
		return Config{}, err
	}
	ragVectorTopK, err := positiveInt("RAG_VECTOR_TOP_K", 15)
	if err != nil {
		return Config{}, err
	}
	ragTitleTopK, err := positiveInt("RAG_TITLE_TOP_K", 10)
	if err != nil {
		return Config{}, err
	}
	ragKeywordTopK, err := positiveInt("RAG_KEYWORD_TOP_K", 15)
	if err != nil {
		return Config{}, err
	}
	ragFusionTopK, err := positiveInt("RAG_FUSION_TOP_K", 20)
	if err != nil {
		return Config{}, err
	}
	ragContextTopK, err := positiveInt("RAG_CONTEXT_PARENT_TOP_K", 5)
	if err != nil {
		return Config{}, err
	}
	if ragContextTopK > ragFusionTopK {
		return Config{}, fmt.Errorf("RAG_CONTEXT_PARENT_TOP_K must not exceed RAG_FUSION_TOP_K")
	}
	rerankMinScore, err := optionalFloat("RAG_RERANK_MIN_SCORE")
	if err != nil {
		return Config{}, err
	}
	rerankMinMargin, err := optionalFloat("RAG_RERANK_MIN_MARGIN")
	if err != nil {
		return Config{}, err
	}
	minQualifiedEvidence, err := positiveInt("RAG_MIN_QUALIFIED_EVIDENCE", 1)
	if err != nil {
		return Config{}, err
	}
	return Config{
		DatabaseURL:             databaseURL,
		MigrationDatabaseURL:    migrationDatabaseURL,
		ListenAddress:           valueOrDefault("LISTEN_ADDRESS", "0.0.0.0:8000"),
		CORSOrigins:             origins,
		TokenTTL:                time.Duration(tokenHours) * time.Hour,
		StatementTimeout:        time.Duration(statementMS) * time.Millisecond,
		PoolSize:                int32(poolSize),
		LogLevel:                parseLogLevel(valueOrDefault("LOG_LEVEL", "INFO")),
		DataDir:                 dataDir,
		MaxAttachmentBytes:      int64(maxAttachmentBytes),
		EmbeddingBaseURL:        valueOrDefault("RAG_EMBEDDING_BASE_URL", "http://llm-gateway:4000/v1"),
		EmbeddingAPIKey:         strings.TrimSpace(os.Getenv("RAG_EMBEDDING_API_KEY")),
		EmbeddingModel:          valueOrDefault("RAG_EMBEDDING_MODEL", "iic/nlp_gte_sentence-embedding_chinese-small"),
		EmbeddingDimensions:     embeddingDimensions,
		EmbeddingSendDimensions: parseBool(valueOrDefault("RAG_EMBEDDING_SEND_DIMENSIONS", "false")),
		RerankBaseURL:           valueOrDefault("RAG_RERANK_BASE_URL", "http://reranker-service:8080"),
		RerankModel:             valueOrDefault("RAG_RERANK_MODEL", "BAAI/bge-reranker-v2-m3"),
		RAGVectorTopK:           ragVectorTopK, RAGTitleTopK: ragTitleTopK, RAGKeywordTopK: ragKeywordTopK,
		RAGFusionTopK: ragFusionTopK, RAGContextTopK: ragContextTopK,
		RAGRerankMinScore: rerankMinScore, RAGRerankMinMargin: rerankMinMargin,
		RAGMinQualifiedEvidence:      minQualifiedEvidence,
		RAGVerifierModel:             valueOrDefault("RAG_VERIFIER_MODEL", valueOrDefault("AI_MODEL", "gpt-5.6")),
		AIAPIKey:                     strings.TrimSpace(os.Getenv("AI_API_KEY")),
		AIBaseURL:                    valueOrDefault("AI_BASE_URL", "https://api.openai.com/v1"),
		AIModel:                      valueOrDefault("AI_MODEL", "gpt-5.6"),
		AISystemPrompt:               valueOrDefault("AI_SYSTEM_PROMPT", "你是一个温暖、贴心的 AI 助手。"),
		Environment:                  valueOrDefault("APP_ENV", "development"),
		ScheduledReportsEnabled:      parseBool(valueOrDefault("SCHEDULED_REPORTS_ENABLED", "true")),
		ScheduledReportPoll:          time.Duration(max(10, pollSeconds)) * time.Second,
		ResearchEnabled:              parseBool(valueOrDefault("RESEARCH_ENABLED", "true")),
		ResearchWorkers:              researchWorkers,
		ResearchMaxKeywords:          researchMaxKeywords,
		ResearchMaxURLs:              researchMaxURLs,
		ResearchMaxResults:           researchMaxResults,
		ResearchMaxImages:            researchMaxImages,
		ResearchMaxImageBytes:        int64(researchMaxImageBytes),
		ResearchMaxBodyChars:         researchMaxBodyChars,
		ResearchLease:                time.Duration(researchLeaseSeconds) * time.Second,
		ResearchMaxAttempts:          researchMaxAttempts,
		ResearchRequestInterval:      time.Duration(researchIntervalMS) * time.Millisecond,
		ResearchHTTPTimeout:          time.Duration(researchTimeoutSeconds) * time.Second,
		ResearchOCRURL:               strings.TrimSpace(os.Getenv("RESEARCH_OCR_URL")),
		XHSSessionEncryptionKey:      strings.TrimSpace(os.Getenv("XHS_SESSION_ENCRYPTION_KEY")),
		XHSSessionKeyVersion:         xhsKeyVersion,
		XHSAuthorizationTTL:          time.Duration(xhsAuthorizationTTLSeconds) * time.Second,
		XHSAuthorizationEnabled:      parseBool(valueOrDefault("XHS_AUTHORIZATION_ENABLED", "false")),
		XHSChromePath:                valueOrDefault("XHS_CHROME_PATH", "/usr/bin/chromium"),
		KnowledgeIndexBatchSize:      knowledgeIndexBatchSize,
		KnowledgeIndexPollSeconds:    knowledgeIndexPollSeconds,
		KnowledgeMaxUploadBytes:      int64(knowledgeMaxUploadBytes),
		KnowledgeMaxExtractedBytes:   int64(knowledgeMaxExtractedBytes),
		KnowledgeMaxFileBytes:        int64(knowledgeMaxFileBytes),
		KnowledgeMaxFiles:            knowledgeMaxFiles,
		KnowledgeMaxDepth:            knowledgeMaxDepth,
		KnowledgeMaxCompressionRatio: knowledgeMaxCompressionRatio,
		RedisURL:                     valueOrDefault("REDIS_URL", "redis://redis:6379/0"),
	}, nil
}

func parseBool(value string) bool {
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}

func normalizeDatabaseURL(value string) string {
	value = strings.Replace(value, "postgresql+psycopg://", "postgresql://", 1)
	if !strings.HasPrefix(value, "postgresql://") && !strings.HasPrefix(value, "postgres://") {
		return ""
	}
	return value
}

func valueOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func splitCSV(value string) []string {
	var values []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			values = append(values, item)
		}
	}
	return values
}

func positiveInt(key string, fallback int) (int, error) {
	raw := valueOrDefault(key, strconv.Itoa(fallback))
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return value, nil
}

func optionalFloat(key string) (*float64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, fmt.Errorf("%s must be a number", key)
	}
	return &value, nil
}

func nonNegativeInt(key string, fallback int) (int, error) {
	raw := valueOrDefault(key, strconv.Itoa(fallback))
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", key)
	}
	return value, nil
}

func parseLogLevel(value string) slog.Level {
	switch strings.ToUpper(value) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
