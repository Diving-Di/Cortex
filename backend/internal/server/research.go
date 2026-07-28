package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/httpx"
	"diary-listener/backend/internal/research"
	"diary-listener/backend/internal/store"
	"github.com/google/uuid"
)

type createResearchJobRequest struct {
	Mode               string   `json:"mode"`
	Keywords           []string `json:"keywords"`
	URLs               []string `json:"urls"`
	TargetCount        int      `json:"target_count"`
	TargetCollectionID *int64   `json:"target_collection_id"`
	IdempotencyKey     string   `json:"idempotency_key"`
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
		values, err := research.ValidateKeywords(request.Keywords, s.cfg.ResearchMaxKeywords)
		if err != nil {
			httpx.WriteError(w, s.logger, apierror.New("RESEARCH_INVALID_KEYWORD", "研究关键词无效", 422))
			return
		}
		payload = map[string]any{"keywords": values}
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
	job, err := s.store.CreateResearchJob(r.Context(), principalFrom(r.Context()), request.Mode,
		raw, request.TargetCount, request.TargetCollectionID, idempotencyKey, s.cfg.ResearchMaxAttempts)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, job)
}

func (s *Server) listResearchJobs(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := researchPagination(r)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	items, total, err := s.store.ListResearchJobs(r.Context(), principalFrom(r.Context()), limit, offset)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (s *Server) getResearchJob(w http.ResponseWriter, r *http.Request) {
	id, err := knowledgePathID(r, "jobID")
	if err == nil {
		var job store.ResearchJob
		job, err = s.store.GetResearchJob(r.Context(), principalFrom(r.Context()), id)
		if err == nil {
			httpx.JSON(w, http.StatusOK, job)
			return
		}
	}
	httpx.WriteError(w, s.logger, err)
}

func (s *Server) cancelResearchJob(w http.ResponseWriter, r *http.Request) {
	id, err := knowledgePathID(r, "jobID")
	if err == nil {
		err = s.store.CancelResearchJob(r.Context(), principalFrom(r.Context()), id)
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]string{"status": "cancel_requested"})
}

func (s *Server) retryResearchJob(w http.ResponseWriter, r *http.Request) {
	id, err := knowledgePathID(r, "jobID")
	if err == nil {
		err = s.store.RetryResearchJob(r.Context(), principalFrom(r.Context()), id)
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
	items, total, err := s.store.ListResearchSources(r.Context(), principalFrom(r.Context()), filter)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (s *Server) getResearchSource(w http.ResponseWriter, r *http.Request) {
	id, err := knowledgePathID(r, "sourceID")
	if err == nil {
		var item store.ResearchSource
		item, err = s.store.GetResearchSource(r.Context(), principalFrom(r.Context()), id)
		if err == nil {
			httpx.JSON(w, http.StatusOK, item)
			return
		}
	}
	httpx.WriteError(w, s.logger, err)
}

func (s *Server) recollectResearchSource(w http.ResponseWriter, r *http.Request) {
	id, err := knowledgePathID(r, "sourceID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	principal := principalFrom(r.Context())
	source, err := s.store.GetResearchSource(r.Context(), principal, id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if source.Status == "saved" {
		httpx.WriteError(w, s.logger, apierror.New("RESEARCH_ALREADY_SAVED", "已保存来源不能直接重新采集", 409))
		return
	}
	raw, _ := json.Marshal(map[string]any{"urls": []string{source.NormalizedURL}})
	job, err := s.store.CreateResearchJob(r.Context(), principal, "urls", raw, 1,
		source.TargetCollectionID, uuid.NewString(), s.cfg.ResearchMaxAttempts)
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
	id, err := knowledgePathID(r, "sourceID")
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
	draft, err := s.store.UpdateResearchDraft(r.Context(), principalFrom(r.Context()), id,
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
	id, err := knowledgePathID(r, "sourceID")
	if err == nil {
		err = s.store.IgnoreResearchSources(r.Context(), principalFrom(r.Context()), []int64{id})
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
	if err := s.store.IgnoreResearchSources(r.Context(), principalFrom(r.Context()), request.IDs); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) saveResearchSource(w http.ResponseWriter, r *http.Request) {
	id, err := knowledgePathID(r, "sourceID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	document, err := s.saveResearchSourceToKnowledge(r, id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, document.Response())
}

func (s *Server) batchSaveResearchSources(w http.ResponseWriter, r *http.Request) {
	var request researchIDsRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if !validResearchIDs(request.IDs) {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	result := make([]store.KnowledgeDocumentResponse, 0, len(request.IDs))
	for _, id := range request.IDs {
		document, err := s.saveResearchSourceToKnowledge(r, id)
		if err != nil {
			httpx.WriteError(w, s.logger, err)
			return
		}
		result = append(result, document.Response())
	}
	httpx.JSON(w, http.StatusAccepted, result)
}

func (s *Server) saveResearchSourceToKnowledge(r *http.Request, id int64) (store.KnowledgeDocument, error) {
	principal := principalFrom(r.Context())
	source, err := s.store.GetResearchSource(r.Context(), principal, id)
	if err != nil {
		return store.KnowledgeDocument{}, err
	}
	if source.Draft == nil {
		return store.KnowledgeDocument{}, apierror.New("RESEARCH_VERSION_CONFLICT", "研究草稿尚未生成", 409)
	}
	if source.Draft.KnowledgeDocumentID != nil {
		return s.store.GetKnowledgeDocument(r.Context(), principal, *source.Draft.KnowledgeDocumentID)
	}
	content := store.ResearchTextFile(source)
	digest := sha256.Sum256([]byte(content))
	now := time.Now().UTC()
	relative := filepath.Join("knowledge", principal.TenantID.String(), now.Format("2006"), now.Format("01"), uuid.NewString()+".txt")
	target, err := s.safeDataPath(relative, "knowledge")
	if err != nil {
		return store.KnowledgeDocument{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
		return store.KnowledgeDocument{}, err
	}
	if err := os.WriteFile(target, []byte(content), 0640); err != nil {
		return store.KnowledgeDocument{}, err
	}
	name := truncateRunes(strings.TrimSpace(source.Title), 200)
	if name == "" {
		name = fmt.Sprintf("小红书研究-%d", source.ID)
	}
	item, err := s.store.AddKnowledgeDocument(r.Context(), principal, store.KnowledgeDocument{
		CollectionID: source.TargetCollectionID, OriginalName: name + ".txt", StoredPath: filepath.ToSlash(relative),
		MIMEType: "text/plain; charset=utf-8", Extension: ".txt",
		Size: int64(len(content)), SHA256: hex.EncodeToString(digest[:]),
	})
	if err != nil {
		_ = os.Remove(target)
		return store.KnowledgeDocument{}, err
	}
	if err := s.store.MarkResearchSourceSaved(r.Context(), principal, source.ID, item.ID); err != nil {
		_ = os.Remove(target)
		return store.KnowledgeDocument{}, err
	}
	return item, nil
}

func (s *Server) downloadResearchAsset(w http.ResponseWriter, r *http.Request) {
	id, err := knowledgePathID(r, "assetID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	item, err := s.store.GetResearchAsset(r.Context(), principalFrom(r.Context()), id)
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
	id, err := knowledgePathID(r, "sourceID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	principal := principalFrom(r.Context())
	source, err := s.store.GetResearchSource(r.Context(), principal, id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if source.Draft != nil && source.Draft.KnowledgeDocumentID != nil {
		document, markErr := s.store.MarkKnowledgeDocumentDeleting(r.Context(), principal, *source.Draft.KnowledgeDocumentID)
		if markErr != nil {
			httpx.WriteError(w, s.logger, markErr)
			return
		}
		if path, pathErr := s.safeDataPath(document.StoredPath, "knowledge"); pathErr == nil {
			_ = os.Remove(path)
		}
	}
	for _, asset := range source.Assets {
		if path, pathErr := s.safeDataPath(asset.StoredPath, "research"); pathErr == nil {
			_ = os.Remove(path)
		}
	}
	if err := s.store.SoftDeleteResearchSource(r.Context(), principal, id); err != nil {
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
