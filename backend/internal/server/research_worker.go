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

	"diary-listener/backend/internal/ai"
	"diary-listener/backend/internal/config"
	"diary-listener/backend/internal/domain"
	"diary-listener/backend/internal/research"
	"diary-listener/backend/internal/store"
	"github.com/google/uuid"
)

type researchPayload struct {
	Keywords []string `json:"keywords"`
	URLs     []string `json:"urls"`
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
	collector := research.Collector{
		Client: research.NewHTTPClient(cfg.ResearchHTTPTimeout), MaxBodyBytes: 4 << 20,
		MaxBodyChars: cfg.ResearchMaxBodyChars, MaxImages: cfg.ResearchMaxImages,
		RequestInterval: cfg.ResearchRequestInterval,
	}
	if cfg.XHSAuthorizationEnabled && strings.TrimSpace(cfg.XHSSessionEncryptionKey) != "" {
		_, session, sessionErr := loadXHSSession(ctx, cfg, database, principal)
		if sessionErr == nil {
			collector.CookieHeader = session.CookieHeader("www.xiaohongshu.com", time.Now())
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
		} else if job.Mode == "keyword" {
			_ = database.CompleteResearchJob(ctx, principal, job.ID, true, "XHS_AUTH_REQUIRED")
			return
		}
	} else if job.Mode == "keyword" {
		_ = database.CompleteResearchJob(ctx, principal, job.ID, true, "XHS_AUTH_NOT_CONFIGURED")
		return
	}
	urls := payload.URLs
	if job.Mode == "keyword" {
		seen := map[string]bool{}
		for _, keyword := range payload.Keywords {
			values, err := collector.Search(ctx, keyword, job.TargetCount)
			if err != nil {
				logger.Warn("research search failed", "job_id", job.ID, "error_code", research.ErrorCode(err))
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
			waitResearch(ctx, cfg.ResearchRequestInterval)
		}
	}
	if len(urls) == 0 {
		_ = database.CompleteResearchJob(ctx, principal, job.ID, true, "XHS_SOURCE_UNAVAILABLE")
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
			_ = database.FailResearchSource(ctx, principal, source.ID, code, research.PublicError(code))
			researchSourcesFailed.Add(1)
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
		content := collected.Content
		if len(ocrTexts) > 0 {
			content += "\n\n【图片提取文字】\n" + strings.Join(ocrTexts, "\n")
		}
		digest := sha256.Sum256([]byte(content))
		contentHash := hex.EncodeToString(digest[:])
		if err := database.SetResearchJobStage(ctx, principal, job.ID, "organizing", cfg.ResearchLease); err != nil {
			return
		}
		organized := organizeResearch(ctx, cfg, principal, collected.Title, content, collected.Tags)
		if err := database.CompleteResearchSource(ctx, principal, source.ID, collected.Title,
			collected.Author, content, contentHash, collected.Tags, organized.Summary,
			organized.KeyPoints, organized.Category, organized.SuggestedTags, cfg.AIModel); err != nil {
			_ = database.FailResearchSource(ctx, principal, source.ID, "RESEARCH_STORE_FAILED", "研究结果保存失败")
			researchSourcesFailed.Add(1)
			continue
		}
		successes++
		researchSourcesCollected.Add(1)
		_ = database.SetResearchJobStage(ctx, principal, job.ID, "collecting", cfg.ResearchLease)
		waitResearch(ctx, cfg.ResearchRequestInterval)
	}
	_ = database.CompleteResearchJob(ctx, principal, job.ID, successes == 0, "XHS_SOURCE_UNAVAILABLE")
	if successes == 0 {
		researchJobsFailed.Add(1)
	} else {
		researchJobsCompleted.Add(1)
	}
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
