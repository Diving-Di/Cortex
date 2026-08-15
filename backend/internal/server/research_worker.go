package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"cortex/backend/internal/ai"
	"cortex/backend/internal/config"
	"cortex/backend/internal/domain"
	"cortex/backend/internal/research"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

type researchPayload struct {
	Keywords   []string `json:"keywords"`
	URLs       []string `json:"urls"`
	SearchSort string   `json:"search_sort"`
}

type researchAIResult struct {
	Summary       string   `json:"summary"`
	KeyPoints     []string `json:"key_points"`
	Category      string   `json:"category"`
	SuggestedTags []string `json:"suggested_tags"`
}

func RunResearchWorkers(ctx context.Context, cfg config.Config, database *store.Store, logger *slog.Logger) {
	if !cfg.ResearchEnabled {
		return
	}
	owner := "research-" + uuid.NewString()
	for index := 0; index < max(1, cfg.ResearchWorkers); index++ {
		go runResearchWorker(ctx, cfg, database, logger, owner)
	}
}

func runResearchWorker(ctx context.Context, cfg config.Config, database *store.Store, logger *slog.Logger, owner string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		jobs, err := database.ClaimResearchJobs(ctx, owner, 1, cfg.ResearchLease)
		if err != nil && ctx.Err() == nil {
			logger.Error("claim research job", "error_code", "RESEARCH_CLAIM_FAILED")
		}
		for _, job := range jobs {
			processResearchJob(ctx, cfg, database, logger, job)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func processResearchJob(ctx context.Context, cfg config.Config, database *store.Store, logger *slog.Logger, job store.ResearchJob) {
	principal := domain.Principal{UserID: job.UserID, TenantID: job.TenantID, TenantActive: true}
	var payload researchPayload
	if err := json.Unmarshal(job.QueryPayload, &payload); err != nil {
		_ = database.CompleteResearchJob(ctx, principal, job.ID, true, "RESEARCH_INVALID_PAYLOAD")
		return
	}
	httpCollector := &research.Collector{
		Client: research.NewHTTPClient(cfg.ResearchHTTPTimeout), MaxBodyBytes: 4 << 20,
		MaxBodyChars: cfg.ResearchMaxBodyChars, MaxImages: cfg.ResearchMaxImages,
		RequestInterval: cfg.ResearchRequestInterval,
	}
	var collector research.SourceCollector = httpCollector
	if cfg.XHSAuthorizationEnabled && strings.TrimSpace(cfg.XHSSessionEncryptionKey) != "" {
		_, session, sessionErr := loadXHSSession(ctx, cfg, database, principal)
		if sessionErr == nil {
			if session.FormatVersion < 2 {
				_ = database.CompleteResearchJob(ctx, principal, job.ID, true, "XHS_REAUTH_REQUIRED")
				return
			}
			logger.Info("research browser session loaded", "job_id", job.ID,
				"local_storage_entries", len(session.LocalStorage))
			httpCollector.CookieHeader = session.CookieHeader("www.xiaohongshu.com", time.Now())
			leaseOwner := "research-session-" + uuid.NewString()
			acquired, leaseErr := database.AcquireXHSAuthorizationLease(ctx, principal, leaseOwner, cfg.ResearchLease)
			if leaseErr != nil || !acquired {
				_ = database.RequeueResearchJob(ctx, principal, job.ID, 5*time.Second)
				return
			}
			defer func() {
				releaseContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = database.ReleaseXHSAuthorizationLease(releaseContext, principal, leaseOwner)
			}()
			browserCollector, browserErr := research.NewBrowserCollector(
				ctx, cfg.XHSChromePath, session, cfg.ResearchMaxImages,
			)
			if browserErr == nil {
				collector = browserCollector
				defer browserCollector.Close()
			} else if job.Mode == "keyword" {
				code := research.ErrorCode(browserErr)
				logger.Warn("research browser unavailable", "job_id", job.ID, "error_code", code)
				_ = database.CompleteResearchJob(ctx, principal, job.ID, true, code)
				return
			}
		} else if job.Mode == "keyword" {
			_ = database.CompleteResearchJob(ctx, principal, job.ID, true, "XHS_AUTH_REQUIRED")
			return
		}
	} else if job.Mode == "keyword" {
		_ = database.CompleteResearchJob(ctx, principal, job.ID, true, "XHS_AUTH_NOT_CONFIGURED")
		return
	}
	urls := payload.URLs
	searchFailureCode := ""
	if job.Mode == "keyword" {
		seen := map[string]bool{}
		for _, keyword := range payload.Keywords {
			values, err := collector.Search(ctx, keyword, job.TargetCount, payload.SearchSort)
			if err != nil {
				code := research.ErrorCode(err)
				searchFailureCode = code
				statuses := []int(nil)
				responseMeta := []string(nil)
				if browser, ok := collector.(*research.BrowserCollector); ok {
					statuses = browser.SearchAPIStatuses()
					responseMeta = browser.SearchResponseMeta()
				}
				logger.Warn("research search failed", "job_id", job.ID, "error_code", code,
					"search_api_statuses", statuses, "search_response_meta", responseMeta)
				if code == "XHS_AUTH_REQUIRED" {
					_ = database.MarkXHSAuthorizationVerified(ctx, principal, false)
				}
				if code == "XHS_RATE_LIMITED" {
					deferRateLimitedResearch(ctx, database, principal, job, code)
					return
				}
				continue
			}
			for _, value := range values {
				if !seen[value] && len(urls) < job.TargetCount {
					seen[value] = true
					urls = append(urls, value)
				}
			}
			if len(urls) >= job.TargetCount {
				break
			}
			waitResearch(ctx, researchJitter(cfg.ResearchRequestInterval, job.ID, len(urls)))
		}
	}
	if len(urls) == 0 {
		if searchFailureCode == "" {
			searchFailureCode = "XHS_SOURCE_UNAVAILABLE"
		}
		_ = database.CompleteResearchJob(ctx, principal, job.ID, true, searchFailureCode)
		return
	}
	successes := 0
	for index, rawURL := range urls {
		if index >= job.TargetCount || ctx.Err() != nil {
			break
		}
		current, err := database.GetResearchJob(ctx, principal, job.ID)
		if err != nil || current.CancelRequestedAt != nil {
			_ = database.CompleteResearchJob(ctx, principal, job.ID, true, "RESEARCH_CANCELLED")
			return
		}
		normalized, err := research.NormalizeURL(rawURL)
		if err != nil {
			continue
		}
		source, err := database.AddResearchSource(ctx, principal, job.ID, rawURL, normalized)
		if err != nil {
			continue
		}
		collected, err := collector.Collect(ctx, normalized)
		if err != nil {
			code := research.ErrorCode(err)
			if browser, ok := collector.(*research.BrowserCollector); ok {
				logger.Warn("research source collection failed", "job_id", job.ID,
					"source_id", source.ID, "error_code", code,
					"page_diagnostics", browser.SearchResponseMeta())
			}
			_ = database.FailResearchSource(ctx, principal, source.ID, code, research.PublicError(code))
			researchSourcesFailed.Add(1)
			if code == "XHS_RATE_LIMITED" {
				deferRateLimitedResearch(ctx, database, principal, job, code)
				return
			}
			continue
		}
		if err := database.SetResearchJobStage(ctx, principal, job.ID, "extracting", cfg.ResearchLease); err != nil {
			return
		}
		var ocrTexts []string
		for position, imageURL := range collected.ImageURLs {
			text, saveErr := saveResearchImage(ctx, cfg, database, principal, source.ID, position, imageURL)
			if saveErr != nil {
				logger.Warn("research image failed", "source_id", source.ID, "error_code", "RESEARCH_ASSET_INVALID")
				continue
			}
			if text != "" {
				ocrTexts = append(ocrTexts, text)
			}
		}
		content, diagnostics := research.PrepareContent(
			collected.Content, ocrTexts, collected.ParseStrategy, len(collected.ImageURLs),
		)
		digest := sha256.Sum256([]byte(content))
		contentHash := hex.EncodeToString(digest[:])
		if err := database.SetResearchJobStage(ctx, principal, job.ID, "organizing", cfg.ResearchLease); err != nil {
			return
		}
		formattedContent, formatStatus := formatResearchContent(ctx, cfg, principal, collected.Title, content)
		diagnostics.FormatStatus = formatStatus
		organized := organizeResearch(ctx, cfg, principal, collected.Title, formattedContent, collected.Tags)
		if err := database.CompleteResearchSource(ctx, principal, source.ID, collected.Title,
			collected.Author, content, formattedContent, contentHash, collected.Tags, collected.PublishedAt,
			collected.LikeCount, collected.CollectCount, collected.CommentCount, organized.Summary,
			organized.KeyPoints, organized.Category, organized.SuggestedTags, cfg.AIModel,
			diagnostics); err != nil {
			_ = database.FailResearchSource(ctx, principal, source.ID, "RESEARCH_STORE_FAILED", "研究结果保存失败")
			researchSourcesFailed.Add(1)
			continue
		}
		successes++
		researchSourcesCollected.Add(1)
		_ = database.SetResearchJobStage(ctx, principal, job.ID, "collecting", cfg.ResearchLease)
		waitResearch(ctx, researchJitter(cfg.ResearchRequestInterval, job.ID, index))
	}
	_ = database.CompleteResearchJob(ctx, principal, job.ID, successes == 0, "XHS_SOURCE_UNAVAILABLE")
	if successes == 0 {
		researchJobsFailed.Add(1)
	} else {
		researchJobsCompleted.Add(1)
	}
}

func formatResearchContent(
	ctx context.Context, cfg config.Config, principal domain.Principal, title, content string,
) (string, string) {
	content = strings.TrimSpace(content)
	if content == "" || utf8.RuneCountInString(content) < 100 {
		return content, "deterministic"
	}
	if strings.TrimSpace(cfg.AIAPIKey) == "" {
		return content, "ai_unavailable"
	}
	client := &ai.EinoClient{
		BaseURL: cfg.AIBaseURL, APIKey: cfg.AIAPIKey,
		HTTPClient: &http.Client{Timeout: 90 * time.Second},
	}
	requestContext := ai.WithRequestMetadata(ctx, ai.RequestMetadata{
		RequestID: uuid.NewString(), RequestType: "research.format",
		Tenant: principal.TenantID.String(), Environment: cfg.Environment,
	})
	prompt := `请清理以下公开内容和 OCR 文本：合并错误断行、保留标题层级与列表、修正明显 OCR 空格问题。` +
		`不得添加、推断或删除事实，不得输出解释，只输出 Markdown。` +
		"\n标题：" + title + "\n内容：\n" + truncateRunes(content, 50_000)
	events, err := client.StreamChat(requestContext, ai.ChatRequest{
		Model: cfg.AIModel, Messages: []ai.Message{
			{Role: "system", Content: "你是 Cortex 的内容格式化器，只清理结构，不改变事实。"},
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return content, "ai_failed"
	}
	var output strings.Builder
	for event := range events {
		if event.Err != nil {
			return content, "ai_failed"
		}
		output.WriteString(event.Content)
		if output.Len() > 100_000 {
			return content, "ai_failed"
		}
	}
	formatted := strings.TrimSpace(output.String())
	formatted = strings.TrimPrefix(formatted, "```markdown")
	formatted = strings.TrimPrefix(formatted, "```")
	formatted = strings.TrimSuffix(formatted, "```")
	formatted = strings.TrimSpace(formatted)
	if formatted == "" {
		return content, "ai_failed"
	}
	return formatted, "ai_formatted"
}

func organizeResearch(ctx context.Context, cfg config.Config, principal domain.Principal, title, content string, tags []string) researchAIResult {
	fallback := researchAIResult{
		Summary:   truncateRunes(strings.TrimSpace(content), 800),
		KeyPoints: []string{}, Category: "待分类", SuggestedTags: tags,
	}
	if strings.TrimSpace(cfg.AIAPIKey) == "" {
		return fallback
	}
	client := &ai.EinoClient{
		BaseURL: cfg.AIBaseURL, APIKey: cfg.AIAPIKey,
		HTTPClient: &http.Client{Timeout: 90 * time.Second},
	}
	requestContext := ai.WithRequestMetadata(ctx, ai.RequestMetadata{
		RequestID: uuid.NewString(), RequestType: "research.organize",
		Tenant: principal.TenantID.String(), Environment: cfg.Environment,
	})
	prompt := `请整理以下公开内容。只输出 JSON 对象，字段为 summary(string)、key_points(string数组，最多8项)、category(string)、suggested_tags(string数组，最多10项)。不要补充来源中没有的事实。` +
		"\n标题：" + title + "\n公开标签：" + strings.Join(tags, ",") + "\n内容：\n" + truncateRunes(content, 50_000)
	events, err := client.StreamChat(requestContext, ai.ChatRequest{
		Model: cfg.AIModel, Messages: []ai.Message{
			{Role: "system", Content: "你是 Cortex 的内容研究整理器。"},
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return fallback
	}
	var output strings.Builder
	for event := range events {
		if event.Err != nil {
			return fallback
		}
		output.WriteString(event.Content)
		if output.Len() > 32_000 {
			return fallback
		}
	}
	raw := strings.TrimSpace(output.String())
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	var result researchAIResult
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &result) != nil || strings.TrimSpace(result.Summary) == "" {
		return fallback
	}
	result.Summary = truncateRunes(strings.TrimSpace(result.Summary), 4000)
	result.Category = truncateRunes(strings.TrimSpace(result.Category), 120)
	result.KeyPoints = cleanStrings(result.KeyPoints, 500)
	result.SuggestedTags = cleanStrings(result.SuggestedTags, 80)
	if len(result.KeyPoints) > 20 {
		result.KeyPoints = result.KeyPoints[:20]
	}
	if len(result.SuggestedTags) > 20 {
		result.SuggestedTags = result.SuggestedTags[:20]
	}
	return result
}

func saveResearchImage(
	ctx context.Context, cfg config.Config, database *store.Store, principal domain.Principal,
	sourceID int64, position int, rawURL string,
) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" {
		return "", errors.New("invalid image URL")
	}
	if err := research.ValidatePublicDestination(ctx, parsed.Hostname(), nil); err != nil {
		return "", err
	}
	client := research.NewHTTPClient(cfg.ResearchHTTPTimeout)
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 || request.URL.Scheme != "https" {
			return errors.New("unsafe redirect")
		}
		return research.ValidatePublicDestination(request.Context(), request.URL.Hostname(), nil)
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	request.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CortexResearch/1.0)")
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", errors.New("image unavailable")
	}
	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	extensions := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}
	extension, ok := extensions[mediaType]
	if !ok {
		return "", errors.New("unsupported image")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, cfg.ResearchMaxImageBytes+1))
	if err != nil || int64(len(body)) == 0 || int64(len(body)) > cfg.ResearchMaxImageBytes {
		return "", errors.New("invalid image size")
	}
	if !validImageSignature(body, mediaType) {
		return "", errors.New("invalid image signature")
	}
	digest := sha256.Sum256(body)
	urlDigest := sha256.Sum256([]byte(rawURL))
	relative := filepath.Join("research", principal.TenantID.String(), fmt.Sprint(sourceID), uuid.NewString()+extension)
	target := filepath.Join(cfg.DataDir, relative)
	root := filepath.Join(cfg.DataDir, "research")
	within, err := filepath.Rel(root, target)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid research path")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
		return "", err
	}
	if err := os.WriteFile(target, body, 0640); err != nil {
		return "", err
	}
	ocrStatus := "unavailable"
	ocrText := ""
	if cfg.ResearchOCRURL != "" {
		ocrText, err = callResearchOCR(ctx, cfg.ResearchOCRURL, mediaType, body)
		if err != nil {
			ocrStatus = "failed"
		} else {
			ocrStatus = "ready"
			ocrText = truncateRunes(ocrText, cfg.ResearchMaxBodyChars)
		}
	}
	_, err = database.AddResearchAsset(ctx, principal, sourceID, position, filepath.ToSlash(relative),
		hex.EncodeToString(urlDigest[:]), mediaType, int64(len(body)), hex.EncodeToString(digest[:]),
		ocrStatus, ocrText)
	if err != nil {
		_ = os.Remove(target)
		return "", err
	}
	return ocrText, nil
}

func callResearchOCR(ctx context.Context, endpoint, mediaType string, body []byte) (string, error) {
	payload, _ := json.Marshal(map[string]any{"mime_type": mediaType, "content": body})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 60 * time.Second}).Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return "", errors.New("OCR unavailable")
	}
	var result struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return "", err
	}
	return result.Text, nil
}

func validImageSignature(body []byte, mediaType string) bool {
	switch mediaType {
	case "image/jpeg":
		return len(body) >= 3 && body[0] == 0xff && body[1] == 0xd8 && body[2] == 0xff
	case "image/png":
		return len(body) >= 8 && bytes.Equal(body[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10})
	case "image/webp":
		return len(body) >= 12 && string(body[:4]) == "RIFF" && string(body[8:12]) == "WEBP"
	default:
		return false
	}
}

func waitResearch(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func researchBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 5 {
		attempt = 5
	}
	return time.Duration(15*(1<<(attempt-1))) * time.Second
}

func researchJitter(base time.Duration, jobID int64, position int) time.Duration {
	if base <= 0 {
		return 0
	}
	// Stable ±20% jitter avoids synchronized workers while keeping tests deterministic.
	bucket := (jobID*31 + int64(position)*17) % 41
	percent := 80 + bucket
	return time.Duration(int64(base) * percent / 100)
}

func deferRateLimitedResearch(
	ctx context.Context, database *store.Store, principal domain.Principal, job store.ResearchJob, code string,
) {
	if job.AttemptCount >= job.MaxAttempts {
		_ = database.CompleteResearchJob(ctx, principal, job.ID, true, code)
		return
	}
	_ = database.DeferResearchJob(ctx, principal, job.ID, researchBackoff(job.AttemptCount), code)
}
