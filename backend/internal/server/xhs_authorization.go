package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/config"
	"cortex/backend/internal/domain"
	"cortex/backend/internal/httpx"
	"cortex/backend/internal/research"
	"cortex/backend/internal/secretbox"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

func (s *Server) getXHSAuthorization(w http.ResponseWriter, r *http.Request) {
	if !s.xhsAuthorizationConfigured() {
		httpx.WriteError(w, s.logger, apierror.New("XHS_AUTH_NOT_CONFIGURED", "小红书授权功能未配置", 503))
		return
	}
	item, err := s.store.GetXHSAuthorization(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	requiresReauthorization := false
	if item.Status == "authorized" {
		_, state, loadErr := s.loadXHSSession(r.Context(), principalFrom(r.Context()))
		requiresReauthorization = loadErr != nil || state.FormatVersion < 2
	}
	httpx.JSON(w, http.StatusOK, struct {
		store.XHSAuthorization
		RequiresReauthorization bool `json:"requires_reauthorization"`
	}{XHSAuthorization: item, RequiresReauthorization: requiresReauthorization})
}

func (s *Server) startXHSAuthorization(w http.ResponseWriter, r *http.Request) {
	if !s.xhsAuthorizationConfigured() {
		httpx.WriteError(w, s.logger, apierror.New("XHS_AUTH_NOT_CONFIGURED", "小红书授权功能未配置", 503))
		return
	}
	item, err := s.store.CreateXHSAuthAttempt(r.Context(), principalFrom(r.Context()),
		time.Now().Add(s.cfg.XHSAuthorizationTTL), s.cfg.XHSSessionKeyVersion)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	researchAuthorizationsStarted.Add(1)
	httpx.JSON(w, http.StatusAccepted, item)
}

func (s *Server) getXHSAuthAttempt(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(strings.TrimSpace(r.PathValue("attemptID")))
	if err != nil {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	item, err := s.store.GetXHSAuthAttempt(r.Context(), principalFrom(r.Context()), id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, item)
}

func (s *Server) getXHSAuthQR(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(strings.TrimSpace(r.PathValue("attemptID")))
	if err != nil {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	item, err := s.store.GetXHSAuthAttempt(r.Context(), principalFrom(r.Context()), id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	active := item.Status == "starting" || item.Status == "waiting_for_scan" ||
		item.Status == "scanned" || item.Status == "verification_required"
	if time.Now().After(item.ExpiresAt) || !active {
		httpx.WriteError(w, s.logger, apierror.New("XHS_QR_EXPIRED", "授权二维码已失效", 410))
		return
	}
	if item.QRPath == nil {
		httpx.WriteError(w, s.logger, apierror.New("XHS_QR_PENDING", "授权二维码正在生成", 404))
		return
	}
	root := filepath.Join(s.cfg.DataDir, "runtime", "xhs-auth")
	path, err := safeRuntimePath(root, *item.QRPath)
	if err != nil {
		httpx.WriteError(w, s.logger, apierror.New("XHS_QR_UNAVAILABLE", "授权二维码不可用", 404))
		return
	}
	content, err := os.ReadFile(path)
	if err != nil {
		httpx.WriteError(w, s.logger, apierror.New("XHS_QR_PENDING", "授权二维码正在生成", 404))
		return
	}
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", fmt.Sprint(len(content)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (s *Server) cancelXHSAuthorization(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(strings.TrimSpace(r.PathValue("attemptID")))
	if err == nil {
		err = s.store.CancelXHSAuthAttempt(r.Context(), principalFrom(r.Context()), id)
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) revokeXHSAuthorization(w http.ResponseWriter, r *http.Request) {
	if err := s.store.RevokeXHSAuthorization(r.Context(), principalFrom(r.Context())); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) verifyXHSAuthorization(w http.ResponseWriter, r *http.Request) {
	item, state, err := s.loadXHSSession(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	request, _ := http.NewRequestWithContext(r.Context(), http.MethodGet,
		"https://www.xiaohongshu.com/explore", nil)
	request.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CortexResearch/1.0)")
	request.Header.Set("Cookie", state.CookieHeader("www.xiaohongshu.com", time.Now()))
	response, err := research.NewHTTPClient(s.cfg.ResearchHTTPTimeout).Do(request)
	valid := err == nil && response.StatusCode >= 200 && response.StatusCode < 400
	if response != nil {
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if readErr != nil {
			valid = false
		} else {
			state := research.DetectPageState(response.Request.URL.String(), "", string(body), valid)
			valid = state == research.PageReady
		}
		_ = response.Body.Close()
	}
	_ = s.store.MarkXHSAuthorizationVerified(r.Context(), principalFrom(r.Context()), valid)
	if !valid {
		httpx.WriteError(w, s.logger, apierror.New("XHS_AUTH_EXPIRED", "小红书授权已失效，请重新扫码", 401))
		return
	}
	item.LastVerifiedAt = timePointer(time.Now())
	httpx.JSON(w, http.StatusOK, item)
}

func (s *Server) xhsAuthorizationConfigured() bool {
	if !s.cfg.XHSAuthorizationEnabled || strings.TrimSpace(s.cfg.XHSSessionEncryptionKey) == "" {
		return false
	}
	_, err := secretbox.New(s.cfg.XHSSessionKeyVersion, s.cfg.XHSSessionEncryptionKey)
	return err == nil
}

func (s *Server) loadXHSSession(ctx context.Context, principal domain.Principal) (store.XHSAuthorization, research.SessionState, error) {
	return loadXHSSession(ctx, s.cfg, s.store, principal)
}

func loadXHSSession(ctx context.Context, cfg config.Config, database *store.Store, principal domain.Principal) (store.XHSAuthorization, research.SessionState, error) {
	var state research.SessionState
	item, err := database.GetXHSAuthorization(ctx, principal)
	if err != nil {
		return item, state, err
	}
	if item.Status != "authorized" || len(item.EncryptedState) == 0 || len(item.EncryptionNonce) == 0 {
		return item, state, apierror.New("XHS_AUTH_REQUIRED", "需要先授权小红书账号", 401)
	}
	box, err := secretbox.New(item.KeyVersion, cfg.XHSSessionEncryptionKey)
	if err != nil {
		return item, state, apierror.New("XHS_SESSION_DECRYPT_FAILED", "小红书授权凭据不可用，请重新授权", 503)
	}
	raw, err := box.Open(item.EncryptedState, item.EncryptionNonce,
		[]byte(xhsAuthorizationAAD(item.TenantID.String(), item.ID, item.KeyVersion)))
	if err != nil || json.Unmarshal(raw, &state) != nil || !state.Authorized() {
		return item, state, apierror.New("XHS_SESSION_DECRYPT_FAILED", "小红书授权凭据不可用，请重新授权", 503)
	}
	return item, state, nil
}

func safeRuntimePath(root, stored string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(filepath.Join(filepath.Dir(filepath.Dir(root)), filepath.FromSlash(stored)))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("unsafe runtime path")
	}
	return path, nil
}

func timePointer(value time.Time) *time.Time { return &value }
