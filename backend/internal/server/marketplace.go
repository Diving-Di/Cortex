package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/httpx"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

type templateCursor struct {
	Version  int       `json:"v"`
	Ranking  string    `json:"r"`
	Score    float64   `json:"s"`
	PublicID uuid.UUID `json:"i"`
}

func (s *Server) encodeTemplateCursor(c templateCursor) string {
	payload, _ := json.Marshal(c)
	mac := hmac.New(sha256.New, []byte(s.cfg.DatabaseURL))
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(append(payload, mac.Sum(nil)...))
}
func (s *Server) decodeTemplateCursor(value, ranking string) (*float64, *uuid.UUID, error) {
	if value == "" {
		return nil, nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) <= sha256.Size {
		return nil, nil, apierror.Validation(nil)
	}
	payload, signature := raw[:len(raw)-sha256.Size], raw[len(raw)-sha256.Size:]
	mac := hmac.New(sha256.New, []byte(s.cfg.DatabaseURL))
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return nil, nil, apierror.Validation(nil)
	}
	var c templateCursor
	if json.Unmarshal(payload, &c) != nil || c.Version != 1 || c.Ranking != ranking || c.PublicID == uuid.Nil {
		return nil, nil, apierror.Validation(nil)
	}
	return &c.Score, &c.PublicID, nil
}

type publicProfileRequest struct {
	Nickname        string `json:"nickname"`
	Discoverable    bool   `json:"discoverable"`
	ExpectedVersion *int   `json:"expected_version"`
}
type templateRequest struct {
	Title           string `json:"title"`
	Description     string `json:"description"`
	ContentMarkdown string `json:"content_markdown"`
	Category        string `json:"category"`
	ExpectedVersion *int   `json:"expected_version"`
}

func (s *Server) getPublicProfile(w http.ResponseWriter, r *http.Request) {
	x, err := s.store.GetPublicProfile(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, 200, x)
}
func (s *Server) upsertPublicProfile(w http.ResponseWriter, r *http.Request) {
	var q publicProfileRequest
	if err := httpx.DecodeJSON(r, &q); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	x, err := s.store.UpsertPublicProfile(r.Context(), principalFrom(r.Context()), q.Nickname, q.Discoverable, q.ExpectedVersion)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, 200, x)
}
func templateInput(q templateRequest) store.TemplateInput {
	return store.TemplateInput{Title: q.Title, Description: q.Description, ContentMarkdown: q.ContentMarkdown, Category: q.Category}
}
func (s *Server) listMyTemplates(w http.ResponseWriter, r *http.Request) {
	x, err := s.store.ListWritingTemplates(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"items": x})
}
func (s *Server) createTemplate(w http.ResponseWriter, r *http.Request) {
	var q templateRequest
	if err := httpx.DecodeJSON(r, &q); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	x, err := s.store.CreateWritingTemplate(r.Context(), principalFrom(r.Context()), templateInput(q))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, 201, x)
}
func templatePathID(r *http.Request) (int64, error) {
	v, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("templateID")), 10, 64)
	if err != nil || v <= 0 {
		return 0, apierror.Validation(nil)
	}
	return v, nil
}
func publicTemplatePathID(r *http.Request) (uuid.UUID, error) {
	v, err := uuid.Parse(strings.TrimSpace(r.PathValue("publicID")))
	if err != nil {
		return uuid.Nil, apierror.Validation(nil)
	}
	return v, nil
}
func (s *Server) getTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := templatePathID(r)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	x, err := s.store.GetWritingTemplate(r.Context(), principalFrom(r.Context()), id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, 200, x)
}
func (s *Server) updateTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := templatePathID(r)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	var q templateRequest
	if e := httpx.DecodeJSON(r, &q); e != nil {
		httpx.WriteError(w, s.logger, e)
		return
	}
	if q.ExpectedVersion == nil {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	x, err := s.store.UpdateWritingTemplate(r.Context(), principalFrom(r.Context()), id, templateInput(q), *q.ExpectedVersion)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, 200, x)
}
func (s *Server) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := templatePathID(r)
	if err == nil {
		err = s.store.DeleteWritingTemplate(r.Context(), principalFrom(r.Context()), id)
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) publishTemplate(w http.ResponseWriter, r *http.Request) {
	if !s.allowUserRequest(r, "template-publish", 5, 24*time.Hour) {
		httpx.WriteError(w, s.logger, apierror.New("RATE_LIMITED", "操作过于频繁", 429))
		return
	}
	id, err := templatePathID(r)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	x, err := s.store.PublishWritingTemplate(r.Context(), principalFrom(r.Context()), id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, 200, x)
}
func (s *Server) withdrawTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := templatePathID(r)
	if err == nil {
		err = s.store.WithdrawWritingTemplate(r.Context(), principalFrom(r.Context()), id)
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) listPublicTemplates(w http.ResponseWriter, r *http.Request) {
	if !s.allowUserRequest(r, "template-list", 60, time.Minute) || !s.allowIPRequest(r, "template-list", 180, time.Minute) {
		httpx.WriteError(w, s.logger, apierror.New("RATE_LIMITED", "请求过于频繁", 429))
		return
	}
	limit, err := positiveQueryInt(r.URL.Query().Get("page_size"), 20, 100)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	ranking := strings.TrimSpace(r.URL.Query().Get("ranking"))
	if ranking == "" {
		ranking = "recommended"
	}
	afterScore, afterID, err := s.decodeTemplateCursor(r.URL.Query().Get("cursor"), ranking)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	var x []store.PublicTemplate
	var scores []float64
	usedRedis := false
	if s.redis != nil && r.URL.Query().Get("query") == "" && r.URL.Query().Get("category") == "" && ranking != "recommended" {
		key, keyOK, keyErr := s.redis.ActiveTemplateRankingKey(r.Context(), ranking, "")
		if ranking == "daily" {
			zone, _ := time.LoadLocation("Asia/Shanghai")
			key, keyOK, keyErr = s.redis.ActiveTemplateRankingKey(r.Context(), ranking, time.Now().In(zone).Format("20060102"))
		}
		if keyErr == nil && keyOK {
			if ranked, cacheErr := s.redis.RankingPage(r.Context(), key, afterScore, limit+1); cacheErr == nil && len(ranked) > 0 {
				ids := make([]uuid.UUID, 0, len(ranked))
				scoreByID := map[uuid.UUID]float64{}
				for _, item := range ranked {
					id, e := uuid.Parse(item.Member)
					if e != nil {
						continue
					}
					if afterScore != nil && afterID != nil && item.Score == *afterScore && id.String() >= afterID.String() {
						continue
					}
					ids = append(ids, id)
					scoreByID[id] = item.Score
					if len(ids) >= limit+1 {
						break
					}
				}
				if fetched, e := s.store.GetPublicTemplatesByIDs(r.Context(), principalFrom(r.Context()), ids); e == nil {
					byID := map[uuid.UUID]store.PublicTemplate{}
					for _, item := range fetched {
						byID[item.PublicID] = item
					}
					for _, id := range ids {
						if item, ok := byID[id]; ok {
							x = append(x, item)
							scores = append(scores, scoreByID[id])
						}
					}
					usedRedis = true
				}
			}
		}
	}
	if !usedRedis {
		x, scores, err = s.store.ListPublicTemplates(r.Context(), principalFrom(r.Context()), r.URL.Query().Get("query"), r.URL.Query().Get("category"), ranking, limit+1, afterScore, afterID)
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	next := ""
	if len(x) > limit {
		x = x[:limit]
		scores = scores[:limit]
		last := len(x) - 1
		next = s.encodeTemplateCursor(templateCursor{Version: 1, Ranking: ranking, Score: scores[last], PublicID: x[last].PublicID})
	}
	httpx.JSON(w, 200, map[string]any{"items": x, "page_size": limit, "ranking": ranking, "next_cursor": next})
}
func (s *Server) getPublicTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := publicTemplatePathID(r)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	principal := principalFrom(r.Context())
	var x store.PublicTemplate
	cacheKey := "cortex:tpl:detail:" + id.String()
	cacheHit := false
	if s.redis != nil {
		if encoded, ok, cacheErr := s.redis.Get(r.Context(), cacheKey); cacheErr == nil && ok {
			if encoded == "__missing__" {
				if _, versionErr := s.store.GetPublicTemplateVersion(r.Context(), id); versionErr != nil {
					httpx.WriteError(w, s.logger, versionErr)
					return
				}
				templateCacheMisses.Add(1)
				_ = s.redis.Delete(r.Context(), cacheKey)
			}
			if json.Unmarshal([]byte(encoded), &x) == nil {
				if currentVersion, versionErr := s.store.GetPublicTemplateVersion(r.Context(), id); versionErr == nil && currentVersion == x.Version {
					cacheHit = true
					templateCacheHits.Add(1)
				} else if versionErr != nil {
					err = versionErr
				} else {
					templateCacheMisses.Add(1)
				}
			}
		} else if cacheErr != nil {
			templateCacheErrors.Add(1)
		} else {
			templateCacheMisses.Add(1)
		}
	}
	if !cacheHit && err == nil {
		x, err = s.store.GetPublicTemplate(r.Context(), principal, id)
		if err == nil && s.redis != nil {
			cached := x
			cached.Liked = false
			cached.Favorited = false
			if encoded, e := json.Marshal(cached); e == nil {
				_ = s.redis.Set(r.Context(), cacheKey, string(encoded), 10*time.Minute+time.Duration(time.Now().UnixNano()%int64(time.Minute)))
			}
		}
	}
	if err != nil {
		var publicErr *apierror.Error
		if s.redis != nil && errors.As(err, &publicErr) && publicErr.Code == "PUBLIC_TEMPLATE_NOT_FOUND" {
			_ = s.redis.Set(r.Context(), cacheKey, "__missing__", 30*time.Second)
		}
		httpx.WriteError(w, s.logger, err)
		return
	}
	if cacheHit {
		x.Liked, x.Favorited, err = s.store.GetTemplateReactions(r.Context(), principal, id)
		if err != nil {
			httpx.WriteError(w, s.logger, err)
			return
		}
	}
	httpx.JSON(w, 200, x)
}
func (s *Server) getPublicTemplateStats(w http.ResponseWriter, r *http.Request) {
	id, err := publicTemplatePathID(r)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	day := strings.TrimSpace(r.URL.Query().Get("day"))
	if day == "" {
		zone, _ := time.LoadLocation("Asia/Shanghai")
		day = time.Now().In(zone).Format("20060102")
	}
	if len(day) != 8 {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	if _, err = time.Parse("20060102", day); err != nil {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	if s.redis == nil {
		httpx.JSON(w, http.StatusOK, map[string]any{"day": day, "unique_visitors_available": false})
		return
	}
	uv, err := s.redis.TemplateUniqueVisitors(r.Context(), id.String(), day)
	if err != nil {
		httpx.JSON(w, http.StatusOK, map[string]any{"day": day, "unique_visitors_available": false})
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"day": day, "unique_visitors": uv, "unique_visitors_available": true})
}
func (s *Server) templateReaction(w http.ResponseWriter, r *http.Request, kind string, enabled bool) {
	if !s.allowUserRequest(r, "template-reaction", 30, time.Minute) {
		httpx.WriteError(w, s.logger, apierror.New("RATE_LIMITED", "操作过于频繁", 429))
		return
	}
	id, err := publicTemplatePathID(r)
	if err == nil {
		err = s.store.SetTemplateReaction(r.Context(), principalFrom(r.Context()), id, kind, enabled)
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if kind == "like" {
		templatePublicLikes.Add(1)
	} else {
		templatePublicFavorites.Add(1)
	}
	w.WriteHeader(204)
}
func (s *Server) likeTemplate(w http.ResponseWriter, r *http.Request) {
	s.templateReaction(w, r, "like", true)
}
func (s *Server) unlikeTemplate(w http.ResponseWriter, r *http.Request) {
	s.templateReaction(w, r, "like", false)
}
func (s *Server) favoriteTemplate(w http.ResponseWriter, r *http.Request) {
	s.templateReaction(w, r, "favorite", true)
}
func (s *Server) unfavoriteTemplate(w http.ResponseWriter, r *http.Request) {
	s.templateReaction(w, r, "favorite", false)
}
func (s *Server) useTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := publicTemplatePathID(r)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	noteID, err := s.store.UsePublicTemplate(r.Context(), principalFrom(r.Context()), id, r.Header.Get("Idempotency-Key"))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	templatePublicUses.Add(1)
	httpx.JSON(w, 201, map[string]any{"note_id": noteID})
}
func (s *Server) usePrivateTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := templatePathID(r)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	noteID, err := s.store.UseWritingTemplate(r.Context(), principalFrom(r.Context()), id, r.Header.Get("Idempotency-Key"))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"note_id": noteID})
}
func (s *Server) viewTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := publicTemplatePathID(r)
	if err == nil && s.redis != nil {
		principal := principalFrom(r.Context())
		visitor := aiEventReservationMember(principal.TenantID) + ":" + strconv.FormatInt(int64(principal.UserID), 10)
		digest := sha256.Sum256([]byte(visitor))
		accepted, redisErr := s.redis.Once(r.Context(), "cortex:tpl:view:"+id.String()+":"+base64.RawURLEncoding.EncodeToString(digest[:12]), 10*time.Minute)
		if redisErr != nil || !accepted {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		err = s.store.RecordTemplateView(r.Context(), principalFrom(r.Context()), id)
	} else if err == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	templatePublicViews.Add(1)
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) allowUserRequest(r *http.Request, scope string, limit int, window time.Duration) bool {
	p := principalFrom(r.Context())
	digest := sha256.Sum256([]byte(p.TenantID.String() + ":" + strconv.FormatInt(int64(p.UserID), 10)))
	key := "cortex:rate:" + scope + ":" + base64.RawURLEncoding.EncodeToString(digest[:12])
	return s.allowRateKey(r, key, limit, window)
}

func (s *Server) allowIPRequest(r *http.Request, scope string, limit int, window time.Duration) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	digest := sha256.Sum256([]byte(host))
	key := "cortex:rate:" + scope + ":ip:" + base64.RawURLEncoding.EncodeToString(digest[:12])
	return s.allowRateKey(r, key, limit, window)
}

func (s *Server) allowRateKey(r *http.Request, key string, limit int, window time.Duration) bool {
	if s.authRedis != nil {
		if ok, err := s.authRedis.Allow(r.Context(), key, limit, window); err == nil {
			return ok
		}
	}
	s.rateMu.Lock()
	defer s.rateMu.Unlock()
	now := time.Now()
	value := s.localRates[key]
	if value.Started.IsZero() || now.Sub(value.Started) >= window {
		value = localRateWindow{Started: now}
	}
	value.Count++
	s.localRates[key] = value
	if len(s.localRates) > 10000 {
		for k, v := range s.localRates {
			if now.Sub(v.Started) >= window {
				delete(s.localRates, k)
			}
		}
	}
	return value.Count <= limit
}
