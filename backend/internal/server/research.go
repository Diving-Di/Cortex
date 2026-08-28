package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/httpx"
	"cortex/backend/internal/research"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

type createResearchJobRequest struct {
	Mode           string   `json:"mode"`
	Keywords       []string `json:"keywords"`
	URLs           []string `json:"urls"`
	TargetCount    int      `json:"target_count"`
	IdempotencyKey string   `json:"idempotency_key"`
	SearchSort     string   `json:"search_sort"`
}

func (s *Server) createResearchJob(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.ResearchEnabled {
		httpx.WriteError(w, s.logger, apierror.New("XHS_COLLECTOR_UNAVAILABLE", "小红书研究功能未启用", 503))
		return
	}
	var request createResearchJobRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	request.Mode = strings.TrimSpace(request.Mode)
	var payload any
	switch request.Mode {
	case "keyword":
		if !s.xhsAuthorizationConfigured() {
			httpx.WriteError(w, s.logger, apierror.New("XHS_AUTH_NOT_CONFIGURED", "小红书授权功能未配置", 503))
			return
		}
		if _, state, err := s.loadXHSSession(r.Context(), principalFrom(r.Context())); err != nil {
			httpx.WriteError(w, s.logger, err)
			return
		} else if state.FormatVersion < 2 {
			httpx.WriteError(w, s.logger, apierror.New(
				"XHS_REAUTH_REQUIRED", "小红书授权需要重新扫码升级", 409,
			))
			return
		}
		values, err := research.ValidateKeywords(request.Keywords, s.cfg.ResearchMaxKeywords)
		if err != nil {
			httpx.WriteError(w, s.logger, apierror.New("RESEARCH_INVALID_KEYWORD", "研究关键词无效", 422))
			return
		}
		request.SearchSort = research.NormalizeSearchSort(request.SearchSort)
		payload = map[string]any{"keywords": values, "search_sort": request.SearchSort}
	case "urls":
		if len(request.URLs) == 0 || len(request.URLs) > s.cfg.ResearchMaxURLs {
			httpx.WriteError(w, s.logger, apierror.New("RESEARCH_LIMIT_EXCEEDED", "研究链接数量超过限制", 422))
			return
		}
		seen := map[string]bool{}
		values := make([]string, 0, len(request.URLs))
		for _, raw := range request.URLs {
			value, err := research.NormalizeURL(raw)
			if err != nil {
				httpx.WriteError(w, s.logger, apierror.New("RESEARCH_INVALID_URL", "研究链接无效", 422))
				return
			}
			if !seen[value] {
				seen[value] = true
				values = append(values, value)
			}
		}
		payload = map[string]any{"urls": values}
	default:
		httpx.WriteError(w, s.logger, apierror.New("RESEARCH_INVALID_MODE", "研究方式无效", 422))
		return
	}
	if request.TargetCount <= 0 {
		request.TargetCount = 10
	}
	if request.TargetCount > s.cfg.ResearchMaxResults {
		httpx.WriteError(w, s.logger, apierror.New("RESEARCH_LIMIT_EXCEEDED", "目标结果数量超过限制", 422))
		return
	}
	idempotencyKey := strings.TrimSpace(request.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	if len(idempotencyKey) > 128 {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	raw, _ := json.Marshal(payload)
	job, err := s.research.Create(r.Context(), principalFrom(r.Context()), request.Mode,
		raw, request.TargetCount, idempotencyKey, s.cfg.ResearchMaxAttempts)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	researchJobsCreated.Add(1)
	httpx.JSON(w, http.StatusAccepted, job)
}

func (s *Server) listResearchJobs(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := researchPagination(r)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	items, total, err := s.research.Jobs(r.Context(), principalFrom(r.Context()), limit, offset)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if items == nil {
		items = []store.ResearchJob{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (s *Server) getResearchJob(w http.ResponseWriter, r *http.Request) {
	id, err := researchPathID(r, "jobID")
	if err == nil {
		var job store.ResearchJob
		job, err = s.research.Job(r.Context(), principalFrom(r.Context()), id)
		if err == nil {
			httpx.JSON(w, http.StatusOK, job)
			return
		}
	}
	httpx.WriteError(w, s.logger, err)
}

func (s *Server) cancelResearchJob(w http.ResponseWriter, r *http.Request) {
	id, err := researchPathID(r, "jobID")
	if err == nil {
		err = s.research.Cancel(r.Context(), principalFrom(r.Context()), id)
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	researchJobsCancelled.Add(1)
	httpx.JSON(w, http.StatusAccepted, map[string]string{"status": "cancel_requested"})
}

func (s *Server) retryResearchJob(w http.ResponseWriter, r *http.Request) {
	id, err := researchPathID(r, "jobID")
	if err == nil {
		err = s.research.Retry(r.Context(), principalFrom(r.Context()), id)
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}

func (s *Server) listResearchSources(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := researchPagination(r)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	var jobID *int64
	if raw := strings.TrimSpace(r.URL.Query().Get("job_id")); raw != "" {
		value, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || value <= 0 {
			httpx.WriteError(w, s.logger, apierror.Validation(nil))
			return
		}
		jobID = &value
	}
	filter := store.ResearchSourceFilter{
		JobID: jobID, Status: strings.TrimSpace(r.URL.Query().Get("status")),
		Search: strings.TrimSpace(r.URL.Query().Get("search")),
		Sort:   strings.TrimSpace(r.URL.Query().Get("sort")), Limit: limit, Offset: offset,
	}
	items, total, err := s.research.Sources(r.Context(), principalFrom(r.Context()), filter)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if items == nil {
		items = []store.ResearchSource{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (s *Server) getResearchSource(w http.ResponseWriter, r *http.Request) {
	id, err := researchPathID(r, "sourceID")
	if err == nil {
		var item store.ResearchSource
		item, err = s.research.Source(r.Context(), principalFrom(r.Context()), id)
		if err == nil {
			httpx.JSON(w, http.StatusOK, item)
			return
		}
	}
	httpx.WriteError(w, s.logger, err)
}

func (s *Server) recollectResearchSource(w http.ResponseWriter, r *http.Request) {
	id, err := researchPathID(r, "sourceID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	principal := principalFrom(r.Context())
	source, err := s.research.Source(r.Context(), principal, id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if source.Status == "saved" {
		httpx.WriteError(w, s.logger, apierror.New("RESEARCH_ALREADY_SAVED", "已保存来源不能直接重新采集", 409))
		return
	}
	raw, _ := json.Marshal(map[string]any{"urls": []string{source.NormalizedURL}})
	job, err := s.research.Create(r.Context(), principal, "urls", raw, 1,
		uuid.NewString(), s.cfg.ResearchMaxAttempts)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, job)
}

type updateResearchDraftRequest struct {
	Summary       string   `json:"summary"`
	KeyPoints     []string `json:"key_points"`
	Category      string   `json:"category"`
	SuggestedTags []string `json:"suggested_tags"`
	Version       int      `json:"version"`
}

func (s *Server) updateResearchDraft(w http.ResponseWriter, r *http.Request) {
	id, err := researchPathID(r, "sourceID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	var request updateResearchDraftRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	request.Summary = strings.TrimSpace(request.Summary)
	request.Category = strings.TrimSpace(request.Category)
	if request.Version <= 0 || utf8.RuneCountInString(request.Summary) > 4000 ||
		utf8.RuneCountInString(request.Category) > 120 || len(request.KeyPoints) > 20 ||
		len(request.SuggestedTags) > 20 {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	draft, err := s.research.Draft(r.Context(), principalFrom(r.Context()), id,
		request.Version, request.Summary, cleanStrings(request.KeyPoints, 500),
		cleanStrings(request.SuggestedTags, 80), request.Category)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, draft)
}

type researchIDsRequest struct {
	IDs []int64 `json:"ids"`
}

func (s *Server) ignoreResearchSource(w http.ResponseWriter, r *http.Request) {
	id, err := researchPathID(r, "sourceID")
	if err == nil {
		err = s.research.Ignore(r.Context(), principalFrom(r.Context()), []int64{id})
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) batchIgnoreResearchSources(w http.ResponseWriter, r *http.Request) {
	var request researchIDsRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if !validResearchIDs(request.IDs) {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	if err := s.research.Ignore(r.Context(), principalFrom(r.Context()), request.IDs); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) downloadResearchAsset(w http.ResponseWriter, r *http.Request) {
	id, err := researchPathID(r, "assetID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	item, err := s.research.Asset(r.Context(), principalFrom(r.Context()), id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	path, err := s.safeDataPath(item.StoredPath, "research")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		httpx.WriteError(w, s.logger, apierror.New("RESEARCH_ASSET_INVALID", "研究图片文件缺失", 410))
		return
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	w.Header().Set("Content-Type", item.MIMEType)
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": fmt.Sprintf("research-%d", item.ID)}))
	http.ServeContent(w, r, "", stat.ModTime(), file)
}

func (s *Server) deleteResearchSource(w http.ResponseWriter, r *http.Request) {
	id, err := researchPathID(r, "sourceID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	principal := principalFrom(r.Context())
	source, err := s.research.Source(r.Context(), principal, id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	for _, asset := range source.Assets {
		if path, pathErr := s.safeDataPath(asset.StoredPath, "research"); pathErr == nil {
			_ = os.Remove(path)
		}
	}
	if err := s.research.DeleteSource(r.Context(), principal, id); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func researchPagination(r *http.Request) (int, int, error) {
	limit, err := positiveQueryInt(r.URL.Query().Get("limit"), 20, 100)
	if err != nil {
		return 0, 0, err
	}
	offset := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			return 0, 0, apierror.Validation(nil)
		}
	}
	return limit, offset, nil
}

func researchPathID(r *http.Request, name string) (int64, error) {
	raw := strings.TrimSpace(r.PathValue(name))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get(name))
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, apierror.Validation(nil)
	}
	return value, nil
}

func cleanStrings(values []string, maxRunes int) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		item := strings.TrimSpace(value)
		if item == "" {
			continue
		}
		runes := []rune(item)
		if len(runes) > maxRunes {
			item = string(runes[:maxRunes])
		}
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

func validResearchIDs(values []int64) bool {
	if len(values) == 0 || len(values) > 100 {
		return false
	}
	seen := map[int64]bool{}
	for _, value := range values {
		if value <= 0 || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}
