package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"diary-listener/backend/internal/config"
	"diary-listener/backend/internal/domain"
	"diary-listener/backend/internal/research"
	"diary-listener/backend/internal/secretbox"
	"diary-listener/backend/internal/store"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/google/uuid"
)

func RunXHSAuthorizationWorkers(
	ctx context.Context, cfg config.Config, database *store.Store, logger *slog.Logger,
) {
	if !cfg.XHSAuthorizationEnabled {
		return
	}
	box, err := secretbox.New(cfg.XHSSessionKeyVersion, cfg.XHSSessionEncryptionKey)
	if err != nil {
		logger.Error("disable xhs authorization", "error_code", "XHS_SESSION_KEY_INVALID")
		return
	}
	go runXHSAuthorizationWorker(ctx, cfg, database, logger, box, "xhs-auth-"+uuid.NewString())
}

func runXHSAuthorizationWorker(
	ctx context.Context, cfg config.Config, database *store.Store, logger *slog.Logger,
	box *secretbox.Box, owner string,
) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		attempts, err := database.ClaimXHSAuthAttempts(ctx, owner, 1, 30*time.Second)
		if err != nil && ctx.Err() == nil {
			logger.Error("claim xhs authorization", "error_code", "XHS_AUTH_CLAIM_FAILED")
		}
		for _, attempt := range attempts {
			processXHSAuthorization(ctx, cfg, database, logger, box, attempt)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func processXHSAuthorization(
	parent context.Context, cfg config.Config, database *store.Store, logger *slog.Logger,
	box *secretbox.Box, attempt store.XHSAuthAttempt,
) {
	principal := domain.Principal{
		UserID: attempt.UserID, TenantID: attempt.TenantID, TenantActive: true,
	}
	runtimeRoot := filepath.Join(cfg.DataDir, "runtime", "xhs-auth")
	tempDir := filepath.Join(runtimeRoot, attempt.TenantID.String(), attempt.ID.String())
	if err := os.MkdirAll(tempDir, 0700); err != nil {
		_ = database.FailXHSAuthAttempt(parent, principal, attempt.ID, "XHS_BROWSER_UNAVAILABLE")
		return
	}
	defer os.RemoveAll(tempDir)

	options := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(cfg.XHSChromePath),
		chromedp.UserDataDir(tempDir),
		chromedp.Headless,
		chromedp.NoSandbox,
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/125.0.0.0 Safari/537.36"),
	)
	allocator, cancelAllocator := chromedp.NewExecAllocator(parent, options...)
	defer cancelAllocator()
	browserContext, cancelBrowser := chromedp.NewContext(allocator)
	defer cancelBrowser()
	deadline := time.Until(attempt.ExpiresAt)
	if deadline <= 0 {
		_ = database.FailXHSAuthAttempt(parent, principal, attempt.ID, "XHS_AUTH_TIMEOUT")
		return
	}
	browserContext, cancelTimeout := context.WithTimeout(browserContext, deadline)
	defer cancelTimeout()

	if err := chromedp.Run(browserContext,
		network.Enable(),
		chromedp.Navigate("https://www.xiaohongshu.com/"),
		chromedp.Sleep(3*time.Second),
	); err != nil {
		_ = database.FailXHSAuthAttempt(parent, principal, attempt.ID, "XHS_BROWSER_UNAVAILABLE")
		return
	}
	qrRelative := filepath.Join("runtime", "xhs-auth", attempt.TenantID.String(), attempt.ID.String(), "qr.png")
	qrPath := filepath.Join(cfg.DataDir, qrRelative)
	if err := captureAuthorizationScreenshot(browserContext, qrPath); err != nil {
		_ = database.FailXHSAuthAttempt(parent, principal, attempt.ID, "XHS_QR_UNAVAILABLE")
		return
	}
	qrStored := filepath.ToSlash(qrRelative)
	if err := database.UpdateXHSAuthAttempt(parent, principal, attempt.ID, "waiting_for_scan", &qrStored, ""); err != nil {
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	lastScreenshot := time.Now()
	for {
		select {
		case <-browserContext.Done():
			_ = database.FailXHSAuthAttempt(parent, principal, attempt.ID, "XHS_AUTH_TIMEOUT")
			return
		case <-ticker.C:
			current, err := database.GetXHSAuthAttempt(parent, principal, attempt.ID)
			if err != nil || current.Status == "cancelled" {
				return
			}
			cookies, err := network.GetCookies().Do(browserContext)
			if err == nil {
				state := research.SessionState{Cookies: make([]research.SessionCookie, 0, len(cookies))}
				for _, cookie := range cookies {
					state.Cookies = append(state.Cookies, research.SessionCookie{
						Name: cookie.Name, Value: cookie.Value, Domain: cookie.Domain,
						Path: cookie.Path, Expires: float64(cookie.Expires), Secure: cookie.Secure,
						HTTPOnly: cookie.HTTPOnly, SameSite: cookie.SameSite.String(),
					})
				}
				if state.Authorized() {
					raw, marshalErr := json.Marshal(state)
					if marshalErr != nil || len(raw) > 1<<20 {
						_ = database.FailXHSAuthAttempt(parent, principal, attempt.ID, "XHS_SESSION_INVALID")
						return
					}
					aad := xhsAuthorizationAAD(attempt.TenantID.String(), attempt.AuthorizationID, box.Version())
					ciphertext, nonce, encryptErr := box.Seal(raw, []byte(aad))
					if encryptErr != nil {
						_ = database.FailXHSAuthAttempt(parent, principal, attempt.ID, "XHS_SESSION_ENCRYPT_FAILED")
						return
					}
					if err := database.CompleteXHSAuthorization(parent, principal, attempt.ID,
						ciphertext, nonce, box.Version(), "", nil); err != nil {
						logger.Error("complete xhs authorization", "error_code", "XHS_AUTH_STORE_FAILED")
					}
					return
				}
			}
			var pageText string
			_ = chromedp.Run(browserContext, chromedp.Text("body", &pageText, chromedp.ByQuery))
			if strings.Contains(pageText, "安全验证") || strings.Contains(pageText, "滑块") {
				_ = database.UpdateXHSAuthAttempt(parent, principal, attempt.ID, "verification_required", &qrStored, "")
			}
			if time.Since(lastScreenshot) >= 15*time.Second {
				_ = captureAuthorizationScreenshot(browserContext, qrPath)
				lastScreenshot = time.Now()
			}
		}
	}
}

func captureAuthorizationScreenshot(ctx context.Context, path string) error {
	var screenshot []byte
	if err := chromedp.Run(ctx, chromedp.FullScreenshot(&screenshot, 90)); err != nil {
		return err
	}
	if len(screenshot) == 0 || len(screenshot) > 10<<20 {
		return fmt.Errorf("invalid authorization screenshot")
	}
	return os.WriteFile(path, screenshot, 0600)
}

func xhsAuthorizationAAD(tenantID string, authorizationID int64, keyVersion int) string {
	return fmt.Sprintf("xhs-session|%s|%d|%d", tenantID, authorizationID, keyVersion)
}
