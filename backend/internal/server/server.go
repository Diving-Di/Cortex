package server

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"cortex/backend/internal/ai"
	"cortex/backend/internal/apierror"
	"cortex/backend/internal/blobstore"
	"cortex/backend/internal/config"
	"cortex/backend/internal/domain"
	"cortex/backend/internal/httpx"
	"cortex/backend/internal/rediscoord"
	"cortex/backend/internal/searchindex"
	"cortex/backend/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
)

type Server struct {
	cfg                    config.Config
	store                  *store.Store
	logger                 *slog.Logger
	version                string
	redis                  *rediscoord.Client
	authRedis              *rediscoord.Client
	claimRedis             *rediscoord.Client
	blobs                  blobstore.BlobStore
	localBlobs             blobstore.BlobStore
	search                 *searchindex.Elasticsearch
	rateMu                 sync.Mutex
	localRates             map[string]localRateWindow
	aiEventFallbackSlots   chan struct{}
	aiEventClaimSlots      chan struct{}
	authResolveGroup       singleflight.Group
	aiEventBreakerMu       sync.Mutex
	aiEventBreakerFailures int
	aiEventBreakerUntil    time.Time
}

type localRateWindow struct {
	Started time.Time
	Count   int
}

type contextKey int

const principalKey contextKey = 1
const requestIDKey contextKey = 2

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func New(cfg config.Config, db *store.Store, logger *slog.Logger, version string) http.Handler {
	return NewWithDependencies(cfg, db, logger, version, Dependencies{})
}

// Dependencies contains process-scoped clients. Production wiring constructs
// these once and shares them with the HTTP server and background workers.
// Zero values are supported for focused handler tests.
type Dependencies struct {
	Blobs      blobstore.BlobStore
	LocalBlobs blobstore.BlobStore
	Redis      *rediscoord.Client
	Search     *searchindex.Elasticsearch
}

func NewWithDependencies(cfg config.Config, db *store.Store, logger *slog.Logger, version string, deps Dependencies) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	s := &Server{cfg: cfg, store: db, logger: logger, version: version, localRates: make(map[string]localRateWindow), aiEventFallbackSlots: make(chan struct{}, 2), aiEventClaimSlots: make(chan struct{}, max(1, cfg.AIEventClaimConcurrency))}
	s.blobs = deps.Blobs
	s.localBlobs = deps.LocalBlobs
	s.search = deps.Search
	s.redis = deps.Redis
	s.authRedis = deps.Redis
	s.claimRedis = deps.Redis
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(s.requestTracing())
	router.Use(s.cors())
	router.Use(copyGinParamsToRequest())

	s.registerSystemRoutes(router)
	s.registerPublicAPIRoutes(router)

	authenticated := router.Group("/")
	authenticated.Use(s.authenticate())
	{
		authenticated.POST("/api/v1/auth/logout", gin.WrapF(s.logout))
		authenticated.GET("/api/v1/auth/session", gin.WrapF(s.session))

		active := authenticated.Group("/")
		active.Use(s.requireActiveTenant())
		{
			active.GET("/api/v1/tenant", gin.WrapF(s.getTenant))
			active.PATCH("/api/v1/tenant", gin.WrapF(s.updateTenant))
			active.DELETE("/api/v1/tenant", gin.WrapF(s.deleteTenant))
			active.GET("/api/v1/tags", gin.WrapF(s.listTags))
			active.POST("/api/v1/tags", gin.WrapF(s.createTag))
			active.GET("/api/v1/search", gin.WrapF(s.searchNotes))
			active.GET("/api/v1/dashboard", gin.WrapF(s.dashboard))
			active.GET("/api/v1/settings/ai", gin.WrapF(s.aiSettings))
			active.POST("/api/v1/ai/providers", gin.WrapF(s.configureAIProvider))
			active.POST("/api/v1/ai/stream", gin.WrapF(s.streamAI))
			active.POST("/api/v1/ai/organize", gin.WrapF(s.organizeAI))
			active.POST("/api/v1/ai/organize/confirm", gin.WrapF(s.confirmOrganize))
			active.POST("/api/v1/reports/preview", gin.WrapF(s.previewReport))
			active.POST("/api/v1/reports/generate", gin.WrapF(s.generateReport))
			active.POST("/api/v1/reports/confirm", gin.WrapF(s.confirmReport))
			active.GET("/api/v1/reports/:noteID/sources", gin.WrapF(s.reportSourceList))
			active.GET("/api/v1/conversations", gin.WrapF(s.listV1Conversations))
			active.POST("/api/v1/conversations", gin.WrapF(s.createV1Conversation))
			active.GET("/api/v1/conversations/:conversationID", gin.WrapF(s.getV1Conversation))
			active.PATCH("/api/v1/conversations/:conversationID", gin.WrapF(s.renameV1Conversation))
			active.DELETE("/api/v1/conversations/:conversationID", gin.WrapF(s.deleteV1Conversation))
			active.GET("/api/v1/scheduled-reports", gin.WrapF(s.listScheduledReports))
			active.POST("/api/v1/scheduled-reports", gin.WrapF(s.createScheduledReport))
			active.PATCH("/api/v1/scheduled-reports/:taskID", gin.WrapF(s.setScheduledReportStatus))
			active.POST("/api/v1/scheduled-reports/:taskID/retry", gin.WrapF(s.retryScheduledReport))
			active.GET("/api/v1/scheduled-reports/:taskID/runs", gin.WrapF(s.listScheduledReportRuns))
			active.POST("/api/v1/attachments", gin.WrapF(s.uploadAttachment))
			active.POST("/api/v1/knowledge/uploads", gin.WrapF(s.uploadKnowledge))
			active.GET("/api/v1/knowledge/uploads/:uploadID", gin.WrapF(s.getKnowledgeUpload))
			active.GET("/api/v1/knowledge/documents", gin.WrapF(s.listKnowledgeDocuments))
			active.DELETE("/api/v1/knowledge/documents/:documentID", gin.WrapF(s.deleteKnowledgeDocument))
			active.GET("/api/v1/knowledge/documents/:documentID/assets/:assetID", gin.WrapF(s.downloadKnowledgeAsset))
			active.POST("/api/v1/knowledge/documents/:documentID/retry", gin.WrapF(s.retryKnowledgeDocument))
			active.GET("/api/v1/knowledge/collections", gin.WrapF(s.listKnowledgeCollections))
			active.POST("/api/v1/knowledge/collections", gin.WrapF(s.createKnowledgeCollection))
			active.POST("/api/v1/knowledge/chat/stream", gin.WrapF(s.knowledgeChat))
			active.POST("/api/v1/knowledge/requests/:requestID/feedback", gin.WrapF(s.createKnowledgeFeedback))
			active.POST("/api/v1/knowledge/feedback/:feedbackID/promote", gin.WrapF(s.promoteKnowledgeFeedback))
			active.POST("/api/v1/knowledge/eval-datasets/:datasetID/freeze", gin.WrapF(s.freezeKnowledgeEvalDataset))
			active.GET("/api/v1/attachments/note/:noteID", gin.WrapF(s.listAttachments))
			active.GET("/api/v1/attachments/:attachmentID", gin.WrapF(s.downloadAttachment))
			active.DELETE("/api/v1/attachments/:attachmentID", gin.WrapF(s.deleteAttachment))
			active.POST("/api/v1/exports/markdown", gin.WrapF(s.exportMarkdown))
			active.GET("/api/v1/settings/preferences", gin.WrapF(s.getPreferences))
			active.PUT("/api/v1/settings/preferences", gin.WrapF(s.updatePreferences))
			active.POST("/api/v1/research/jobs", gin.WrapF(s.createResearchJob))
			active.GET("/api/v1/research/jobs", gin.WrapF(s.listResearchJobs))
			active.GET("/api/v1/research/jobs/:jobID", gin.WrapF(s.getResearchJob))
			active.POST("/api/v1/research/jobs/:jobID/cancel", gin.WrapF(s.cancelResearchJob))
			active.POST("/api/v1/research/jobs/:jobID/retry", gin.WrapF(s.retryResearchJob))
			active.GET("/api/v1/research/sources", gin.WrapF(s.listResearchSources))
			active.GET("/api/v1/research/sources/:sourceID", gin.WrapF(s.getResearchSource))
			active.DELETE("/api/v1/research/sources/:sourceID", gin.WrapF(s.deleteResearchSource))
			active.POST("/api/v1/research/sources/:sourceID/retry", gin.WrapF(s.recollectResearchSource))
			active.POST("/api/v1/research/sources/:sourceID/recollect", gin.WrapF(s.recollectResearchSource))
			active.PATCH("/api/v1/research/sources/:sourceID/draft", gin.WrapF(s.updateResearchDraft))
			active.POST("/api/v1/research/sources/:sourceID/ignore", gin.WrapF(s.ignoreResearchSource))
			active.POST("/api/v1/research/sources/batch-ignore", gin.WrapF(s.batchIgnoreResearchSources))
			active.GET("/api/v1/research/assets/:assetID", gin.WrapF(s.downloadResearchAsset))
			active.GET("/api/v1/research/xhs/authorization", gin.WrapF(s.getXHSAuthorization))
			active.POST("/api/v1/research/xhs/authorizations", gin.WrapF(s.startXHSAuthorization))
			active.GET("/api/v1/research/xhs/authorizations/:attemptID", gin.WrapF(s.getXHSAuthAttempt))
			active.GET("/api/v1/research/xhs/authorizations/:attemptID/qr", gin.WrapF(s.getXHSAuthQR))
			active.POST("/api/v1/research/xhs/authorizations/:attemptID/cancel", gin.WrapF(s.cancelXHSAuthorization))
			active.POST("/api/v1/research/xhs/authorization/verify", gin.WrapF(s.verifyXHSAuthorization))
			active.DELETE("/api/v1/research/xhs/authorization", gin.WrapF(s.revokeXHSAuthorization))
			active.GET("/api/v1/public-profile", gin.WrapF(s.getPublicProfile))
			active.PUT("/api/v1/public-profile", gin.WrapF(s.upsertPublicProfile))
			active.GET("/api/v1/templates/public", gin.WrapF(s.listPublicTemplates))
			active.GET("/api/v1/templates/public/:publicID", gin.WrapF(s.getPublicTemplate))
			active.GET("/api/v1/templates/public/:publicID/stats", gin.WrapF(s.getPublicTemplateStats))
			active.GET("/api/v1/templates/mine", gin.WrapF(s.listMyTemplates))
			active.POST("/api/v1/templates", gin.WrapF(s.createTemplate))
			active.GET("/api/v1/templates/:templateID", gin.WrapF(s.getTemplate))
			active.PATCH("/api/v1/templates/:templateID", gin.WrapF(s.updateTemplate))
			active.DELETE("/api/v1/templates/:templateID", gin.WrapF(s.deleteTemplate))
			active.POST("/api/v1/templates/:templateID/publish", gin.WrapF(s.publishTemplate))
			active.POST("/api/v1/templates/:templateID/withdraw", gin.WrapF(s.withdrawTemplate))
			active.POST("/api/v1/templates/:templateID/use", gin.WrapF(s.usePrivateTemplate))
			active.PUT("/api/v1/templates/public/:publicID/like", gin.WrapF(s.likeTemplate))
			active.DELETE("/api/v1/templates/public/:publicID/like", gin.WrapF(s.unlikeTemplate))
			active.PUT("/api/v1/templates/public/:publicID/favorite", gin.WrapF(s.favoriteTemplate))
			active.DELETE("/api/v1/templates/public/:publicID/favorite", gin.WrapF(s.unfavoriteTemplate))
			active.POST("/api/v1/templates/public/:publicID/use", gin.WrapF(s.useTemplate))
			active.POST("/api/v1/templates/public/:publicID/views", gin.WrapF(s.viewTemplate))
			active.GET("/api/v1/ai-points/balance", gin.WrapF(s.aiPointBalance))
			active.GET("/api/v1/ai-events/current", gin.WrapF(s.currentAIEvent))
			active.GET("/api/v1/ai-events/page", gin.WrapF(s.aiEventPage))
			active.GET("/api/v1/ai-events/history", gin.WrapF(s.aiEventHistory))
			active.GET("/api/v1/ai-events/:eventID", gin.WrapF(s.getAIEvent))
			active.GET("/api/v1/ai-events/:eventID/claims/me", gin.WrapF(s.myAIEventClaim))
			active.GET("/api/v1/ai-event-claims/:claimID", gin.WrapF(s.getAIEventClaim))

			notes := active.Group("/api/v1/notes")
			notes.GET("", gin.WrapF(s.listNotes))
			notes.GET("/", gin.WrapF(s.listNotes))
			notes.POST("", gin.WrapF(s.createNote))
			notes.POST("/", gin.WrapF(s.createNote))
			notes.GET("/:noteID", gin.WrapF(s.getNote))
			notes.PATCH("/:noteID", gin.WrapF(s.updateNote))
			notes.DELETE("/:noteID", gin.WrapF(s.deleteNote))
			notes.GET("/:noteID/revisions", gin.WrapF(s.listRevisions))
			notes.POST("/:noteID/revisions/:revisionID/restore", gin.WrapF(s.restoreRevision))
			notes.GET("/:noteID/tags", gin.WrapF(s.listNoteTags))
			notes.PUT("/:noteID/tags", gin.WrapF(s.assignNoteTags))
			notes.PATCH("/:noteID/knowledge", gin.WrapF(s.setNoteKnowledge))
		}
	}
	return router
}

func (s *Server) requestTracing() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if !requestIDPattern.MatchString(requestID) {
			requestID = uuid.NewString()
		}
		c.Header("X-Request-ID", requestID)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestIDKey, requestID))
		started := time.Now()
		c.Next()
		s.logger.Info("http request", "request_id", requestID, "method", c.Request.Method,
			"path", c.Request.URL.Path, "status", c.Writer.Status(), "duration_ms", time.Since(started).Milliseconds())
	}
}

func (s *Server) aiContext(ctx context.Context, requestType string, principal domain.Principal) context.Context {
	requestID, _ := ctx.Value(requestIDKey).(string)
	if requestID == "" {
		requestID = uuid.NewString()
	}
	return ai.WithRequestMetadata(ctx, ai.RequestMetadata{
		RequestID: requestID, RequestType: requestType,
		Tenant: principal.TenantID.String(), Environment: s.cfg.Environment,
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if s.store == nil || s.store.Ping(ctx) != nil {
		httpx.JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) dependencyHealth(w http.ResponseWriter, r *http.Request) {
	status := map[string]string{"database": "unavailable", "storage": "disabled", "redis": "disabled", "search": "disabled", "ai": "disabled"}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if s.store != nil && s.store.Ping(ctx) == nil {
		status["database"] = "available"
	}
	if s.blobs != nil {
		status["storage"] = "unavailable"
		if s.blobs.Ready(ctx) == nil {
			status["storage"] = "available"
		}
	}
	if s.redis != nil {
		status["redis"] = "configured"
	}
	if s.search != nil {
		status["search"] = "configured"
	}
	if s.cfg.AIAPIKey != "" && s.cfg.AIBaseURL != "" {
		status["ai"] = "configured"
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"status": "ok", "dependencies": status})
}

func (s *Server) authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		scheme, token, hasHeader := strings.Cut(
			strings.TrimSpace(c.Request.Header.Get("Authorization")), " ",
		)
		validHeader := hasHeader &&
			(strings.ToLower(scheme) == "token" || strings.ToLower(scheme) == "bearer") &&
			strings.TrimSpace(token) != ""
		usedCookie := false
		if !validHeader {
			token = ""
			cookie, err := c.Request.Cookie(authCookieName)
			if err == nil {
				token = cookie.Value
				usedCookie = true
			}
		}
		if strings.TrimSpace(token) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"detail": "Authentication credentials were not provided.",
			})
			return
		}
		principal, err := s.resolvePrincipal(c.Request.Context(), strings.TrimSpace(token))
		if err != nil {
			var appErr *apierror.Error
			if errors.As(err, &appErr) && appErr.StatusCode == http.StatusUnauthorized {
				if usedCookie {
					s.clearAuthCookie(c.Writer, c.Request)
				}
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"detail": "Invalid or expired token.",
				})
				return
			}
			httpx.WriteError(c.Writer, s.logger, err)
			c.Abort()
			return
		}
		requestContext := context.WithValue(c.Request.Context(), principalKey, principal)
		c.Request = c.Request.WithContext(requestContext)
		c.Next()
	}
}

const invalidPrincipalCache = "__invalid__"

func authCacheKey(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return "cortex:auth:principal:" + base64.RawURLEncoding.EncodeToString(digest[:])
}

func tenantAuthVersionKey(id uuid.UUID) string { return "cortex:auth:tenant-version:" + id.String() }

func (s *Server) resolvePrincipal(ctx context.Context, raw string) (domain.Principal, error) {
	started := time.Now()
	defer func() { observeAIEventStage(&aiEventClaimAuthNanos, &aiEventClaimAuthCount, started) }()
	key := authCacheKey(raw)
	if s.authRedis != nil {
		if encoded, ok, err := s.authRedis.Get(ctx, key); err == nil && ok {
			if encoded == invalidPrincipalCache {
				return domain.Principal{}, apierror.New("AUTHENTICATION_REQUIRED", "Invalid or expired token.", 401)
			}
			var p domain.Principal
			if json.Unmarshal([]byte(encoded), &p) == nil && time.Now().Before(p.TokenExpiresAt) {
				version, exists, versionErr := s.authRedis.Get(ctx, tenantAuthVersionKey(p.TenantID))
				if versionErr == nil && exists && version == fmt.Sprint(p.TenantVersion) {
					p.AuthCacheKey = key
					return p, nil
				}
			}
		}
	}
	resolved, err, _ := s.authResolveGroup.Do(key, func() (any, error) {
		return s.store.ResolvePrincipal(ctx, raw)
	})
	if err != nil {
		if s.authRedis != nil {
			_ = s.authRedis.Set(ctx, key, invalidPrincipalCache, 15*time.Second)
		}
		return domain.Principal{}, err
	}
	p := resolved.(domain.Principal)
	p.AuthCacheKey = key
	if s.authRedis != nil {
		_ = s.authRedis.Set(ctx, tenantAuthVersionKey(p.TenantID), fmt.Sprint(p.TenantVersion), 24*time.Hour)
		if encoded, marshalErr := json.Marshal(p); marshalErr == nil {
			ttl := min(5*time.Minute, time.Until(p.TokenExpiresAt))
			if ttl > 0 {
				_ = s.authRedis.Set(ctx, key, string(encoded), ttl)
			}
		}
	}
	return p, nil
}

func (s *Server) preAuthClaimIPLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.allowIPRequest(c.Request, "ai-event-claim-anonymous", s.cfg.AIEventClaimIPLimit, time.Minute) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"code": "RATE_LIMITED", "message": "请求过于频繁"})
			return
		}
		c.Next()
	}
}

func (s *Server) requireActiveTenant() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !principalFrom(c.Request.Context()).TenantActive {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"detail": "Personal tenant is unavailable.",
			})
			return
		}
		c.Next()
	}
}

func (s *Server) cors() gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(s.cfg.CORSOrigins))
	for _, origin := range s.cfg.CORSOrigins {
		allowed[origin] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if _, ok := allowed[origin]; ok {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-Request-ID")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			if origin != "" {
				if _, ok := allowed[origin]; !ok {
					c.AbortWithStatus(http.StatusForbidden)
					return
				}
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func copyGinParamsToRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, param := range c.Params {
			c.Request.SetPathValue(param.Key, param.Value)
		}
		c.Next()
	}
}

func principalFrom(ctx context.Context) domain.Principal {
	principal, _ := ctx.Value(principalKey).(domain.Principal)
	return principal
}

// Handlers for user preferences (marketplace personalization only).
func (s *Server) getPreferences(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r.Context())
	prefs, err := s.store.GetUserPreferences(r.Context(), principal)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"marketplace_personalization": prefs.MarketplacePersonalization,
		"version":                     prefs.Version,
	})
}

func (s *Server) updatePreferences(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r.Context())
	var body struct {
		Version                    int  `json:"version"`
		MarketplacePersonalization bool `json:"marketplace_personalization"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	prefs, err := s.store.UpdateUserPreferences(r.Context(), principal, body.MarketplacePersonalization, body.Version)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"marketplace_personalization": prefs.MarketplacePersonalization,
		"version":                     prefs.Version,
	})
}
