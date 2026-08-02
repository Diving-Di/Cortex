package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"diary-listener/backend/internal/ai"
	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/config"
	"diary-listener/backend/internal/domain"
	"diary-listener/backend/internal/httpx"
	"diary-listener/backend/internal/recipe"
	"diary-listener/backend/internal/rediscoord"
	"diary-listener/backend/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Server struct {
	cfg        config.Config
	store      *store.Store
	logger     *slog.Logger
	version    string
	redis      *rediscoord.Client
	rateMu     sync.Mutex
	localRates map[string]localRateWindow
}

type localRateWindow struct {
	Started time.Time
	Count   int
}

type contextKey int

const principalKey contextKey = 1
const requestIDKey contextKey = 2

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func New(cfg config.Config, db *store.Store, logger *slog.Logger, version string) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	s := &Server{cfg: cfg, store: db, logger: logger, version: version, localRates: make(map[string]localRateWindow)}
	s.redis, _ = rediscoord.New(cfg.RedisURL)
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
			active.GET("/api/v1/recipes/today", gin.WrapF(s.getTodayRecipe))
			active.POST("/api/v1/recipes/chat", gin.WrapF(s.recipesChat))
			active.GET("/api/v1/recipes/messages/:messageID/sources", gin.WrapF(s.recipeSourceList))
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
			active.POST("/api/v1/templates/public/:publicID/reports", gin.WrapF(s.reportTemplate))
			active.GET("/api/v1/ai-points/balance", gin.WrapF(s.aiPointBalance))
			active.GET("/api/v1/ai-events/current", gin.WrapF(s.currentAIEvent))
			active.GET("/api/v1/ai-events/history", gin.WrapF(s.aiEventHistory))
			active.GET("/api/v1/ai-events/:eventID", gin.WrapF(s.getAIEvent))
			active.POST("/api/v1/ai-events/:eventID/claims", gin.WrapF(s.claimAIEvent))
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

// Handlers for recipes and preferences (basic implementations)
func (s *Server) getTodayRecipe(w http.ResponseWriter, r *http.Request) {
	// determine timezone
	tz := r.URL.Query().Get("timezone")
	principal := principalFrom(r.Context())
	// load user prefs for timezone and restrictions if available
	prefs, err := s.store.GetUserPreferences(r.Context(), principal, s.cfg.RecipeDefaultTimezone)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if tz == "" && prefs.Timezone != "" {
		tz = prefs.Timezone
	}
	if tz == "" {
		tz = s.cfg.RecipeDefaultTimezone
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		httpx.WriteError(w, s.logger, apierror.New("INVALID_TIMEZONE", "invalid timezone", 400))
		return
	}
	now := time.Now().In(loc)
	localDate := now.Format("2006-01-02")

	// get corpus revision
	rev, _ := s.store.LatestRecipeCorpusRevision(r.Context())

	// build candidate list
	restrictions := prefs.DietaryRestrictions
	ids, err := s.store.ListEligibleRecipeIDs(r.Context(), restrictions)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if len(ids) == 0 {
		httpx.WriteError(w, s.logger, apierror.New("RECIPE_NO_ELIGIBLE_DISH", "no eligible dish for current restrictions", 404))
		return
	}

	// deterministic selection: compute min SHA-256(seed + "\n" + recipe_id)
	seedBase := fmt.Sprintf("%d\n%s\n%s\n", principal.UserID, localDate, rev)
	// compute restrictions hash
	rhash := ""
	if len(restrictions) > 0 {
		h := sha256.Sum256([]byte(strings.Join(restrictions, ",")))
		rhash = hex.EncodeToString(h[:])
	}
	seedBase = seedBase + rhash
	var chosen int64 = ids[0]
	var minDigest [32]byte
	first := true
	for _, id := range ids {
		data := fmt.Sprintf("%s\n%d", seedBase, id)
		d := sha256.Sum256([]byte(data))
		if first || bytes.Compare(d[:], minDigest[:]) < 0 {
			minDigest = d
			chosen = id
			first = false
		}
	}

	// load basic fields for chosen id
	var title, category, summary string
	var difficulty, caloriesText *string
	var ingredients []string
	err = s.store.WithTx(r.Context(), func(tx pgx.Tx) error {
		return tx.QueryRow(r.Context(), `SELECT title,category,summary,difficulty,calories_text,ingredients FROM recipe_documents WHERE id=$1`, chosen).
			Scan(&title, &category, &summary, &difficulty, &caloriesText, &ingredients)
	})
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"local_date":      localDate,
		"timezone":        tz,
		"corpus_revision": rev,
		"recipe": map[string]any{
			"id":               chosen,
			"title":            title,
			"category":         category,
			"summary":          summary,
			"difficulty":       difficulty,
			"calories_text":    caloriesText,
			"ingredients":      ingredients,
			"dietary_warnings": []string{},
		},
		"suggested_questions": []string{
			"需要哪些食材和用量？",
			"请完整说明制作步骤。",
			"有哪些容易忽略的技巧？",
		},
	})
}

func (s *Server) recipesChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Question         string `json:"question"`
		RequestID        string `json:"request_id"`
		ConversationID   *int32 `json:"conversation_id"`
		FeaturedRecipeID *int64 `json:"featured_recipe_id"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	req.Question = strings.TrimSpace(req.Question)
	if len([]rune(req.Question)) < 1 || len([]rune(req.Question)) > 5000 || (req.RequestID != "" && !requestIDPattern.MatchString(req.RequestID)) {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	principal := principalFrom(r.Context())

	retriever := recipe.Retriever{
		Store:          s.store,
		RerankURL:      s.cfg.RerankBaseURL + "/rerank",
		RerankModel:    s.cfg.RerankModel,
		EmbeddingURL:   strings.TrimRight(s.cfg.EmbeddingBaseURL, "/") + "/embeddings",
		EmbeddingModel: s.cfg.EmbeddingModel,
		VectorTopK:     s.cfg.RAGVectorTopK, TitleTopK: s.cfg.RAGTitleTopK,
		KeywordTopK: s.cfg.RAGKeywordTopK, FusionTopK: s.cfg.RAGFusionTopK,
		ContextTopK: s.cfg.RAGContextTopK,
	}
	retrievalQuery := req.Question
	var featured *store.RecipeCandidate
	featuredOnly := false
	if req.FeaturedRecipeID != nil {
		candidate, err := s.store.GetRecipeCandidate(r.Context(), *req.FeaturedRecipeID)
		if err != nil {
			httpx.WriteError(w, s.logger, err)
			return
		}
		featured = &candidate
		rewrite := recipe.RewriteQuery(req.Question, candidate.Title)
		retrievalQuery = rewrite.Query
		featuredOnly = rewrite.FeaturedOnly
	}

	var candidates []store.RecipeCandidate
	var err error
	if featuredOnly && featured != nil {
		candidates = []store.RecipeCandidate{*featured}
	} else {
		candidates, err = retriever.Search(r.Context(), retrievalQuery, 10)
		if err != nil {
			httpx.WriteError(w, s.logger, apierror.New("RECIPE_EMBEDDING_UNAVAILABLE", "菜谱检索服务暂时不可用", 503))
			return
		}
		if len(candidates) == 0 {
			httpx.WriteError(w, s.logger, apierror.New("RECIPE_NO_EVIDENCE", "没有找到相关菜谱", 404))
			return
		}
		candidates, err = retriever.Rerank(r.Context(), retrievalQuery, candidates)
		if err != nil {
			httpx.WriteError(w, s.logger, apierror.New("RECIPE_RERANK_UNAVAILABLE", "菜谱精排服务暂时不可用", 503))
			return
		}
	}

	if featured != nil && !featuredOnly {
		filtered := []store.RecipeCandidate{*featured}
		for _, candidate := range candidates {
			if candidate.DocumentID != featured.DocumentID {
				filtered = append(filtered, candidate)
			}
			if len(filtered) == 5 {
				break
			}
		}
		candidates = filtered
	}

	// convert to evidence
	evidence := make([]ai.KnowledgeEvidence, 0, len(candidates))
	for i, c := range candidates {
		evidence = append(evidence, ai.KnowledgeEvidence{
			Citation: fmt.Sprintf("R%d", i+1), Title: c.Title, Kind: "recipe_document",
			Content: c.Snippet, Heading: "",
		})
	}

	conversationContext := s.recipeConversationContext(r, principal, req.ConversationID)
	preferences, err := s.store.GetUserPreferences(r.Context(), principal, s.cfg.RecipeDefaultTimezone)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	events, err := s.aiWorkflow().AnswerKnowledge(s.aiContext(r.Context(), "recipe_chat", principal), ai.KnowledgeInput{
		Question: req.Question, ConversationContext: conversationContext, Evidence: evidence,
		DietaryRestrictions: preferences.DietaryRestrictions,
	})
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}

	apiSources := make([]domain.Source, 0, len(candidates))
	for i, c := range candidates {
		snippet := c.Snippet
		apiSources = append(apiSources, domain.Source{Type: "recipe_document", ID: c.DocumentID, Title: c.Title, Snippet: &snippet, Rank: i + 1})
	}

	s.writeRecipeSSE(w, r, req.Question, events, apiSources, func(ctx context.Context, answer string) (int32, int32, error) {
		// save using recipe-specific store API
		messageID, conversationID, err := s.store.SaveRecipeAnswer(ctx, principal, req.ConversationID, req.RequestID, req.Question, answer, candidates)
		return messageID, conversationID, err
	})
}

func (s *Server) recipeSourceList(w http.ResponseWriter, r *http.Request) {
	messageID, err := pathID(r, "messageID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	result, err := s.store.GetRecipeSources(r.Context(), principalFrom(r.Context()), messageID)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (s *Server) getPreferences(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r.Context())
	prefs, err := s.store.GetUserPreferences(r.Context(), principal, s.cfg.RecipeDefaultTimezone)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"dietary_restrictions":        prefs.DietaryRestrictions,
		"timezone":                    prefs.Timezone,
		"marketplace_personalization": prefs.MarketplacePersonalization,
		"version":                     prefs.Version,
	})
}

func (s *Server) updatePreferences(w http.ResponseWriter, r *http.Request) {
	principal := principalFrom(r.Context())
	var body struct {
		DietaryRestrictions        []string `json:"dietary_restrictions"`
		Timezone                   string   `json:"timezone"`
		Version                    int      `json:"version"`
		MarketplacePersonalization bool     `json:"marketplace_personalization"`
	}
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	body.DietaryRestrictions = recipe.NormalizeDietaryTerms(body.DietaryRestrictions)
	if len(body.DietaryRestrictions) > 50 {
		httpx.WriteError(w, s.logger, apierror.Validation(map[string]any{"dietary_restrictions": "too many items"}))
		return
	}
	for _, term := range body.DietaryRestrictions {
		if len([]rune(term)) > 40 {
			httpx.WriteError(w, s.logger, apierror.Validation(map[string]any{"dietary_restrictions": "item too long"}))
			return
		}
	}
	if _, err := time.LoadLocation(body.Timezone); err != nil {
		httpx.WriteError(w, s.logger, apierror.New("INVALID_TIMEZONE", "invalid timezone", 400))
		return
	}
	prefs, err := s.store.UpdateUserPreferences(r.Context(), principal, body.DietaryRestrictions, body.Timezone, body.MarketplacePersonalization, body.Version)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"dietary_restrictions":        prefs.DietaryRestrictions,
		"timezone":                    prefs.Timezone,
		"marketplace_personalization": prefs.MarketplacePersonalization,
		"version":                     prefs.Version,
	})
}
