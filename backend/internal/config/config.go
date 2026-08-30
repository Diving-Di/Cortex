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
	RuntimeRole                  string
	DatabaseURL                  string
	MigrationDatabaseURL         string
	ListenAddress                string
	CORSOrigins                  []string
	TokenTTL                     time.Duration
	TrustProxyHeaders            bool
	AuthLoginIPLimit             int
	AuthLoginAccountLimit        int
	AuthRegisterIPLimit          int
	AuthTokenIPLimit             int
	StatementTimeout             time.Duration
	PoolSize                     int32
	AuthPoolSize                 int32
	LogLevel                     slog.Level
	DataDir                      string
	StorageBackend               string
	MinIOEndpoint                string
	MinIOBucket                  string
	MinIOAccessKey               string
	MinIOSecretKey               string
	MinIOSecure                  bool
	EventBus                     string
	KafkaRESTURL                 string
	KafkaClientID                string
	RAGRetrievalBackend          string
	ElasticsearchURLs            []string
	ElasticsearchUsername        string
	ElasticsearchPassword        string
	ElasticsearchIndexAlias      string
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
	RAGPlannerEnabled            bool
	RAGPlannerMaxSubqueries      int
	AIAPIKey                     string
	AIBaseURL                    string
	AIModel                      string
	AISystemPrompt               string
	Environment                  string
	ScheduledReportsEnabled      bool
	ScheduledReportPoll          time.Duration
	KnowledgeIndexBatchSize      int
	KnowledgeIndexPollSeconds    int
	KnowledgeMaxUploadBytes      int64
	KnowledgeMaxExtractedBytes   int64
	KnowledgeMaxFileBytes        int64
	KnowledgeMaxFiles            int
	KnowledgeMaxDepth            int
	KnowledgeMaxCompressionRatio int
	DocumentParserURL            string
	DocumentParserTimeout        time.Duration
	RedisURL                     string
	AIEventBuildBatchSize        int
	AIEventBuildLease            time.Duration
	AIEventClaimIPLimit          int
	AIEventClaimConcurrency      int
	AIEventClaimQueueTimeout     time.Duration
}

func Load() (Config, error) {
	runtimeRole := strings.ToLower(valueOrDefault("CORTEX_RUNTIME_ROLE", "all"))
	if runtimeRole != "all" && runtimeRole != "api" && runtimeRole != "worker" {
		return Config{}, fmt.Errorf("CORTEX_RUNTIME_ROLE must be all, api, or worker")
	}
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
	authLoginIPLimit, err := positiveInt("AUTH_LOGIN_IP_LIMIT", 30)
	if err != nil {
		return Config{}, err
	}
	authLoginAccountLimit, err := positiveInt("AUTH_LOGIN_ACCOUNT_LIMIT", 10)
	if err != nil {
		return Config{}, err
	}
	authRegisterIPLimit, err := positiveInt("AUTH_REGISTER_IP_LIMIT", 5)
	if err != nil {
		return Config{}, err
	}
	authTokenIPLimit, err := positiveInt("AUTH_TOKEN_IP_LIMIT", 15)
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
	authPoolSize, err := positiveInt("AUTH_DB_POOL_SIZE", 32)
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
	storageBackend := strings.ToLower(valueOrDefault("STORAGE_BACKEND", "local"))
	if storageBackend != "local" && storageBackend != "minio" {
		return Config{}, fmt.Errorf("STORAGE_BACKEND must be local or minio")
	}
	minioEndpoint, minioBucket := strings.TrimSpace(os.Getenv("MINIO_ENDPOINT")), valueOrDefault("MINIO_BUCKET", "cortex-private")
	minioAccessKey, minioSecretKey := strings.TrimSpace(os.Getenv("MINIO_ACCESS_KEY")), strings.TrimSpace(os.Getenv("MINIO_SECRET_KEY"))
	if storageBackend == "minio" && (minioEndpoint == "" || minioAccessKey == "" || minioSecretKey == "") {
		return Config{}, fmt.Errorf("MinIO configuration is required when STORAGE_BACKEND=minio")
	}
	eventBus := strings.ToLower(valueOrDefault("EVENT_BUS", "postgres"))
	if eventBus != "postgres" && eventBus != "kafka" {
		return Config{}, fmt.Errorf("EVENT_BUS must be postgres or kafka")
	}
	kafkaRESTURL := strings.TrimRight(strings.TrimSpace(os.Getenv("KAFKA_REST_URL")), "/")
	if eventBus == "kafka" && kafkaRESTURL == "" {
		return Config{}, fmt.Errorf("KAFKA_REST_URL is required when EVENT_BUS=kafka")
	}
	retrievalBackend := strings.ToLower(valueOrDefault("RAG_RETRIEVAL_BACKEND", "postgres"))
	if retrievalBackend != "postgres" && retrievalBackend != "elasticsearch" {
		return Config{}, fmt.Errorf("RAG_RETRIEVAL_BACKEND must be postgres or elasticsearch")
	}
	esURLs := splitCSV(os.Getenv("ELASTICSEARCH_URLS"))
	if retrievalBackend == "elasticsearch" && len(esURLs) == 0 {
		return Config{}, fmt.Errorf("ELASTICSEARCH_URLS is required when RAG_RETRIEVAL_BACKEND=elasticsearch")
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
	documentParserTimeoutSeconds, err := positiveInt("DOCUMENT_PARSER_TIMEOUT_SECONDS", 120)
	if err != nil {
		return Config{}, err
	}
	aiEventProjectionBuildBatchSize, err := positiveInt("AI_EVENT_PROJECTION_BUILD_BATCH_SIZE", 250)
	if err != nil {
		return Config{}, err
	}
	aiEventProjectionBuildLeaseSeconds, err := positiveInt("AI_EVENT_PROJECTION_BUILD_LEASE_SECONDS", 60)
	if err != nil {
		return Config{}, err
	}
	aiEventClaimIPLimit, err := positiveInt("AI_EVENT_CLAIM_IP_LIMIT", 60)
	if err != nil {
		return Config{}, err
	}
	aiEventClaimConcurrency, err := positiveInt("AI_EVENT_CLAIM_CONCURRENCY", 16)
	if err != nil {
		return Config{}, err
	}
	aiEventClaimQueueTimeoutMS, err := positiveInt("AI_EVENT_CLAIM_QUEUE_TIMEOUT_MS", 10000)
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
	ragKeywordTopK, err := positiveInt("RAG_KEYWORD_TOP_K", 5)
	if err != nil {
		return Config{}, err
	}
	ragFusionTopK, err := positiveInt("RAG_FUSION_TOP_K", 20)
	if err != nil {
		return Config{}, err
	}
	ragContextTopK, err := positiveInt("RAG_CONTEXT_PARENT_TOP_K", 4)
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
	plannerMaxSubqueries, err := positiveInt("RAG_PLANNER_MAX_SUBQUERIES", 4)
	if err != nil || plannerMaxSubqueries > 4 {
		return Config{}, fmt.Errorf("RAG_PLANNER_MAX_SUBQUERIES must be between 1 and 4")
	}
	return Config{
		RuntimeRole:           runtimeRole,
		DatabaseURL:           databaseURL,
		MigrationDatabaseURL:  migrationDatabaseURL,
		ListenAddress:         valueOrDefault("LISTEN_ADDRESS", "0.0.0.0:8000"),
		CORSOrigins:           origins,
		TokenTTL:              time.Duration(tokenHours) * time.Hour,
		TrustProxyHeaders:     parseBool(valueOrDefault("TRUST_PROXY_HEADERS", "false")),
		AuthLoginIPLimit:      authLoginIPLimit,
		AuthLoginAccountLimit: authLoginAccountLimit,
		AuthRegisterIPLimit:   authRegisterIPLimit,
		AuthTokenIPLimit:      authTokenIPLimit,
		StatementTimeout:      time.Duration(statementMS) * time.Millisecond,
		PoolSize:              int32(poolSize),
		AuthPoolSize:          int32(authPoolSize),
		LogLevel:              parseLogLevel(valueOrDefault("LOG_LEVEL", "INFO")),
		DataDir:               dataDir,
		StorageBackend:        storageBackend, MinIOEndpoint: minioEndpoint, MinIOBucket: minioBucket,
		MinIOAccessKey: minioAccessKey, MinIOSecretKey: minioSecretKey, MinIOSecure: parseBool(valueOrDefault("MINIO_SECURE", "false")),
		EventBus: eventBus, KafkaRESTURL: kafkaRESTURL, KafkaClientID: valueOrDefault("KAFKA_CLIENT_ID", "cortex"),
		RAGRetrievalBackend: retrievalBackend, ElasticsearchURLs: esURLs, ElasticsearchUsername: strings.TrimSpace(os.Getenv("ELASTICSEARCH_USERNAME")), ElasticsearchPassword: strings.TrimSpace(os.Getenv("ELASTICSEARCH_PASSWORD")), ElasticsearchIndexAlias: valueOrDefault("ELASTICSEARCH_INDEX_ALIAS", "cortex-knowledge-read"),
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
		RAGPlannerEnabled:            parseBool(valueOrDefault("RAG_PLANNER_ENABLED", "false")),
		RAGPlannerMaxSubqueries:      plannerMaxSubqueries,
		AIAPIKey:                     strings.TrimSpace(os.Getenv("AI_API_KEY")),
		AIBaseURL:                    valueOrDefault("AI_BASE_URL", "https://api.openai.com/v1"),
		AIModel:                      valueOrDefault("AI_MODEL", "gpt-5.6"),
		AISystemPrompt:               valueOrDefault("AI_SYSTEM_PROMPT", "你是一个温暖、贴心的 AI 助手。"),
		Environment:                  valueOrDefault("APP_ENV", "development"),
		ScheduledReportsEnabled:      parseBool(valueOrDefault("SCHEDULED_REPORTS_ENABLED", "true")),
		ScheduledReportPoll:          time.Duration(max(10, pollSeconds)) * time.Second,
		KnowledgeIndexBatchSize:      knowledgeIndexBatchSize,
		KnowledgeIndexPollSeconds:    knowledgeIndexPollSeconds,
		KnowledgeMaxUploadBytes:      int64(knowledgeMaxUploadBytes),
		KnowledgeMaxExtractedBytes:   int64(knowledgeMaxExtractedBytes),
		KnowledgeMaxFileBytes:        int64(knowledgeMaxFileBytes),
		KnowledgeMaxFiles:            knowledgeMaxFiles,
		KnowledgeMaxDepth:            knowledgeMaxDepth,
		KnowledgeMaxCompressionRatio: knowledgeMaxCompressionRatio,
		DocumentParserURL:            strings.TrimRight(strings.TrimSpace(os.Getenv("DOCUMENT_PARSER_URL")), "/"),
		DocumentParserTimeout:        time.Duration(documentParserTimeoutSeconds) * time.Second,
		RedisURL:                     valueOrDefault("REDIS_URL", "redis://redis:6379/0"),
		AIEventBuildBatchSize:        aiEventProjectionBuildBatchSize,
		AIEventBuildLease:            time.Duration(aiEventProjectionBuildLeaseSeconds) * time.Second,
		AIEventClaimIPLimit:          aiEventClaimIPLimit,
		AIEventClaimConcurrency:      aiEventClaimConcurrency,
		AIEventClaimQueueTimeout:     time.Duration(aiEventClaimQueueTimeoutMS) * time.Millisecond,
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
