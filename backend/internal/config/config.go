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
	DatabaseURL             string
	MigrationDatabaseURL    string
	ListenAddress           string
	CORSOrigins             []string
	TokenTTL                time.Duration
	StatementTimeout        time.Duration
	PoolSize                int32
	LogLevel                slog.Level
	DataDir                 string
	MaxAttachmentBytes      int64
	EmbeddingBaseURL        string
	EmbeddingAPIKey         string
	EmbeddingModel          string
	EmbeddingDimensions     int
	EmbeddingSendDimensions bool
	RerankBaseURL           string
	RerankModel             string
	AIAPIKey                string
	AIBaseURL               string
	AIModel                 string
	AISystemPrompt          string
	Environment             string
	ScheduledReportsEnabled bool
	ScheduledReportPoll     time.Duration
	ResearchEnabled         bool
	ResearchWorkers         int
	ResearchMaxKeywords     int
	ResearchMaxURLs         int
	ResearchMaxResults      int
	ResearchMaxImages       int
	ResearchMaxImageBytes   int64
	ResearchMaxBodyChars    int
	ResearchLease           time.Duration
	ResearchMaxAttempts     int
	ResearchRequestInterval time.Duration
	ResearchHTTPTimeout     time.Duration
	ResearchOCRURL          string
	XHSSessionEncryptionKey string
	XHSSessionKeyVersion    int
	XHSAuthorizationTTL     time.Duration
	XHSAuthorizationEnabled bool
	XHSChromePath           string
	RecipeDefaultTimezone   string
	RecipeIndexWorkers      int
	RecipeIndexBatchSize    int
	RecipeIndexPollSeconds  int
	RedisURL                string
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
	dataDir, err := filepath.Abs(valueOrDefault("DIARY_DATA_DIR", "./data"))
	if err != nil {
		return Config{}, fmt.Errorf("resolve DIARY_DATA_DIR: %w", err)
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
	recipeIndexWorkers, err := positiveInt("RECIPE_INDEX_WORKERS", 1)
	if err != nil {
		return Config{}, err
	}
	recipeIndexBatchSize, err := positiveInt("RECIPE_INDEX_BATCH_SIZE", 16)
	if err != nil {
		return Config{}, err
	}
	recipeIndexPollSeconds, err := positiveInt("RECIPE_INDEX_POLL_SECONDS", 5)
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
		AIAPIKey:                strings.TrimSpace(os.Getenv("AI_API_KEY")),
		AIBaseURL:               valueOrDefault("AI_BASE_URL", "https://api.openai.com/v1"),
		AIModel:                 valueOrDefault("AI_MODEL", "gpt-5.6"),
		AISystemPrompt:          valueOrDefault("AI_SYSTEM_PROMPT", "你是一个温暖、贴心的 AI 助手。"),
		Environment:             valueOrDefault("APP_ENV", "development"),
		ScheduledReportsEnabled: parseBool(valueOrDefault("SCHEDULED_REPORTS_ENABLED", "true")),
		ScheduledReportPoll:     time.Duration(max(10, pollSeconds)) * time.Second,
		ResearchEnabled:         parseBool(valueOrDefault("RESEARCH_ENABLED", "true")),
		ResearchWorkers:         researchWorkers,
		ResearchMaxKeywords:     researchMaxKeywords,
		ResearchMaxURLs:         researchMaxURLs,
		ResearchMaxResults:      researchMaxResults,
		ResearchMaxImages:       researchMaxImages,
		ResearchMaxImageBytes:   int64(researchMaxImageBytes),
		ResearchMaxBodyChars:    researchMaxBodyChars,
		ResearchLease:           time.Duration(researchLeaseSeconds) * time.Second,
		ResearchMaxAttempts:     researchMaxAttempts,
		ResearchRequestInterval: time.Duration(researchIntervalMS) * time.Millisecond,
		ResearchHTTPTimeout:     time.Duration(researchTimeoutSeconds) * time.Second,
		ResearchOCRURL:          strings.TrimSpace(os.Getenv("RESEARCH_OCR_URL")),
		XHSSessionEncryptionKey: strings.TrimSpace(os.Getenv("XHS_SESSION_ENCRYPTION_KEY")),
		XHSSessionKeyVersion:    xhsKeyVersion,
		XHSAuthorizationTTL:     time.Duration(xhsAuthorizationTTLSeconds) * time.Second,
		XHSAuthorizationEnabled: parseBool(valueOrDefault("XHS_AUTHORIZATION_ENABLED", "false")),
		XHSChromePath:           valueOrDefault("XHS_CHROME_PATH", "/usr/bin/chromium"),
		RecipeDefaultTimezone:   valueOrDefault("RECIPE_DEFAULT_TIMEZONE", "Asia/Shanghai"),
		RecipeIndexWorkers:      recipeIndexWorkers,
		RecipeIndexBatchSize:    recipeIndexBatchSize,
		RecipeIndexPollSeconds:  recipeIndexPollSeconds,
		RedisURL:                valueOrDefault("REDIS_URL", "redis://redis:6379/0"),
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
