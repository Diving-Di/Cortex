package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"diary-listener/backend/internal/ai"
	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/config"
	"diary-listener/backend/internal/domain"
	"diary-listener/backend/internal/httpx"
	"diary-listener/backend/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Server struct {
	cfg     config.Config
	store   *store.Store
	logger  *slog.Logger
	version string
}

type contextKey int

const principalKey contextKey = 1
const requestIDKey contextKey = 2

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func New(cfg config.Config, db *store.Store, logger *slog.Logger, version string) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	s := &Server{cfg: cfg, store: db, logger: logger, version: version}
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(s.requestTracing())
	router.Use(s.cors())
	router.Use(copyGinParamsToRequest())

	router.GET("/healthz", gin.WrapF(s.health))
	router.GET("/readyz", gin.WrapF(s.ready))
	router.GET("/metrics", gin.WrapF(s.metrics))
	router.POST("/api/v1/auth/register", gin.WrapF(s.register))
	router.POST("/api/v1/auth/login", gin.WrapF(s.login))

	authenticated := router.Group("/")
	authenticated.Use(s.authenticate())
	{
		authenticated.POST("/api/v1/auth/logout", gin.WrapF(s.logout))
		authenticated.POST("/api/v1/tenant/restore", gin.WrapF(s.restoreTenant))

		active := authenticated.Group("/")
		active.Use(s.requireActiveTenant())
		{
			active.GET("/api/v1/tenant", gin.WrapF(s.getTenant))
			active.PATCH("/api/v1/tenant", gin.WrapF(s.updateTenant))
			active.DELETE("/api/v1/tenant", gin.WrapF(s.deleteTenant))
			active.GET("/api/v1/tags", gin.WrapF(s.listTags))
			active.POST("/api/v1/tags", gin.WrapF(s.createTag))
			active.GET("/api/v1/search", gin.WrapF(s.searchNotes))
			active.GET("/api/dashboard", gin.WrapF(s.dashboard))
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
			active.GET("/api/v1/attachments/note/:noteID", gin.WrapF(s.listAttachments))
			active.GET("/api/v1/attachments/:attachmentID", gin.WrapF(s.downloadAttachment))
			active.DELETE("/api/v1/attachments/:attachmentID", gin.WrapF(s.deleteAttachment))
			active.POST("/api/v1/exports/markdown", gin.WrapF(s.exportMarkdown))
			active.GET("/api/v1/backups/full", gin.WrapF(s.exportFullBackup))
			active.POST("/api/v1/backups/full/restore", gin.WrapF(s.restoreFullBackup))
			active.GET("/api/v1/knowledge/collections", gin.WrapF(s.listKnowledgeCollections))
			active.POST("/api/v1/knowledge/collections", gin.WrapF(s.createKnowledgeCollection))
			active.DELETE("/api/v1/knowledge/collections/:collectionID", gin.WrapF(s.deleteKnowledgeCollection))
			active.GET("/api/v1/knowledge/documents", gin.WrapF(s.listKnowledgeDocuments))
			active.POST("/api/v1/knowledge/documents", gin.WrapF(s.uploadKnowledgeDocument))
			active.GET("/api/v1/knowledge/documents/:documentID", gin.WrapF(s.getKnowledgeDocument))
			active.GET("/api/v1/knowledge/documents/:documentID/download", gin.WrapF(s.downloadKnowledgeDocument))
			active.GET("/api/v1/knowledge/documents/:documentID/preview", gin.WrapF(s.previewKnowledgeDocument))
			active.POST("/api/v1/knowledge/documents/:documentID/reindex", gin.WrapF(s.reindexKnowledgeDocument))
			active.DELETE("/api/v1/knowledge/documents/:documentID", gin.WrapF(s.deleteKnowledgeDocument))
			active.POST("/api/v1/knowledge/chat", gin.WrapF(s.knowledgeChat))
			active.GET("/api/v1/knowledge/messages/:messageID/sources", gin.WrapF(s.knowledgeSourceList))
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
			active.POST("/api/v1/research/sources/:sourceID/save", gin.WrapF(s.saveResearchSource))
			active.POST("/api/v1/research/sources/:sourceID/ignore", gin.WrapF(s.ignoreResearchSource))
			active.POST("/api/v1/research/sources/batch-save", gin.WrapF(s.batchSaveResearchSources))
			active.POST("/api/v1/research/sources/batch-ignore", gin.WrapF(s.batchIgnoreResearchSources))
			active.GET("/api/v1/research/assets/:assetID", gin.WrapF(s.downloadResearchAsset))
			active.GET("/api/v1/research/xhs/authorization", gin.WrapF(s.getXHSAuthorization))
			active.POST("/api/v1/research/xhs/authorizations", gin.WrapF(s.startXHSAuthorization))
			active.GET("/api/v1/research/xhs/authorizations/:attemptID", gin.WrapF(s.getXHSAuthAttempt))
			active.GET("/api/v1/research/xhs/authorizations/:attemptID/qr", gin.WrapF(s.getXHSAuthQR))
			active.POST("/api/v1/research/xhs/authorizations/:attemptID/cancel", gin.WrapF(s.cancelXHSAuthorization))
			active.POST("/api/v1/research/xhs/authorization/verify", gin.WrapF(s.verifyXHSAuthorization))
			active.DELETE("/api/v1/research/xhs/authorization", gin.WrapF(s.revokeXHSAuthorization))

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
	if err := s.store.Ping(ctx); err != nil {
		httpx.JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		scheme, token, ok := strings.Cut(
			strings.TrimSpace(c.Request.Header.Get("Authorization")), " ",
		)
		if !ok ||
			(strings.ToLower(scheme) != "token" && strings.ToLower(scheme) != "bearer") ||
			strings.TrimSpace(token) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"detail": "Authentication credentials were not provided.",
			})
			return
		}
		principal, err := s.store.ResolvePrincipal(c.Request.Context(), strings.TrimSpace(token))
		if err != nil {
			var appErr *apierror.Error
			if errors.As(err, &appErr) && appErr.StatusCode == http.StatusUnauthorized {
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
