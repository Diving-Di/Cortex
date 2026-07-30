package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"diary-listener/backend/internal/config"
	"diary-listener/backend/internal/domain"
	"diary-listener/backend/internal/research"
	"diary-listener/backend/internal/secretbox"
	"diary-listener/backend/internal/store"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
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
	if err := captureAuthorizationQRCode(browserContext, qrPath); err != nil {
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
	cookieReadFailureLogged := false
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
			browserExecutor := cdp.WithExecutor(browserContext, chromedp.FromContext(browserContext).Browser)
			cookies, err := storage.GetCookies().Do(browserExecutor)
			if err != nil && !cookieReadFailureLogged {
				logger.Error("read xhs authorization cookies", "error_code", "XHS_COOKIE_READ_FAILED")
				cookieReadFailureLogged = true
			}
			if err == nil {
				state := research.SessionState{
					FormatVersion: 2,
					Cookies:       make([]research.SessionCookie, 0, len(cookies)),
				}
				for _, cookie := range cookies {
					state.Cookies = append(state.Cookies, research.SessionCookie{
						Name: cookie.Name, Value: cookie.Value, Domain: cookie.Domain,
						Path: cookie.Path, Expires: float64(cookie.Expires), Secure: cookie.Secure,
						HTTPOnly: cookie.HTTPOnly, SameSite: cookie.SameSite.String(),
					})
				}
				if state.Authorized() && xhsPageShowsAuthenticated(browserContext) {
					var localStorage []research.SessionStorageEntry
					if storageErr := chromedp.Run(browserContext, chromedp.Evaluate(
						`Object.entries(window.localStorage).map(([name,value])=>({name,value}))`,
						&localStorage,
					)); storageErr == nil {
						state.LocalStorage = research.SanitizeLocalStorage(localStorage)
					}
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
			var snapshot struct {
				URL   string `json:"url"`
				Title string `json:"title"`
				Text  string `json:"text"`
			}
			_ = chromedp.Run(browserContext, chromedp.Evaluate(`({
				url:location.href,title:document.title,text:(document.body?.innerText||"").slice(0,20000)
			})`, &snapshot))
			if research.DetectPageState(snapshot.URL, snapshot.Title, snapshot.Text, false) ==
				research.PageVerificationRequired {
				_ = database.UpdateXHSAuthAttempt(parent, principal, attempt.ID, "verification_required", &qrStored, "")
			}
			if time.Since(lastScreenshot) >= 15*time.Second {
				_ = captureAuthorizationQRCode(browserContext, qrPath)
				lastScreenshot = time.Now()
			}
		}
	}
}

func xhsPageShowsAuthenticated(ctx context.Context) bool {
	var authenticated bool
	const expression = `(()=>{
		const visible=element=>{
			if(!element)return false;
			const rect=element.getBoundingClientRect(),style=getComputedStyle(element);
			return rect.width>0&&rect.height>0&&style.display!=="none"&&
				style.visibility!=="hidden"&&Number(style.opacity)!==0;
		};
		const loginElements=Array.from(document.querySelectorAll(
			".login-modal,.login-container,.side-bar-component.login-btn"
		));
		return !loginElements.some(visible);
	})()`
	if err := chromedp.Run(ctx, chromedp.Evaluate(expression, &authenticated)); err != nil {
		return false
	}
	return authenticated
}

func captureAuthorizationQRCode(ctx context.Context, path string) error {
	var screenshot []byte
	var found bool
	const markQRCode = `(() => {
		const visibleSquare = (element) => {
			const rect = element.getBoundingClientRect();
			const style = getComputedStyle(element);
			return rect.width >= 80 && rect.width <= 800 &&
				rect.height >= 80 && rect.height <= 800 &&
				Math.max(rect.width, rect.height) / Math.min(rect.width, rect.height) <= 1.35 &&
				style.display !== 'none' && style.visibility !== 'hidden' && Number(style.opacity) !== 0;
		};
		const hint = (element) => {
			let current = element;
			for (let depth = 0; current && depth < 4; depth++, current = current.parentElement) {
				const value = [
					current.id,
					typeof current.className === 'string' ? current.className : '',
					current.getAttribute && current.getAttribute('src'),
					current.getAttribute && current.getAttribute('aria-label'),
				].filter(Boolean).join(' ').toLowerCase();
				if (/qr|qrcode|qr-code|二维码|扫码/.test(value)) return true;
			}
			return false;
		};
		const candidates = Array.from(document.querySelectorAll('img, canvas, svg'))
			.filter(visibleSquare);
		let target = candidates.find((element) => hint(element));
		if (!target) {
			target = candidates.find((element) => {
				const container = element.closest('[role="dialog"], [class*="login"], [class*="modal"]');
				return container && /扫码|二维码/.test(container.textContent || '');
			});
		}
		if (!target) return false;
		document.querySelectorAll('[data-diary-xhs-qr]').forEach((element) =>
			element.removeAttribute('data-diary-xhs-qr'));
		target.setAttribute('data-diary-xhs-qr', 'true');
		return true;
	})()`
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(markQRCode, &found),
	); err != nil || !found {
		return fmt.Errorf("xhs login qr element not found")
	}
	if err := chromedp.Run(ctx,
		chromedp.Screenshot(`[data-diary-xhs-qr="true"]`, &screenshot, chromedp.ByQuery),
	); err != nil {
		return err
	}
	if len(screenshot) == 0 || len(screenshot) > 10<<20 {
		return fmt.Errorf("invalid authorization qr image")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(screenshot))
	if err != nil || config.Width < 80 || config.Height < 80 ||
		config.Width > 1000 || config.Height > 1000 ||
		float64(max(config.Width, config.Height))/float64(min(config.Width, config.Height)) > 1.35 {
		return fmt.Errorf("invalid authorization qr dimensions")
	}
	return os.WriteFile(path, screenshot, 0600)
}

func xhsAuthorizationAAD(tenantID string, authorizationID int64, keyVersion int) string {
	return fmt.Sprintf("xhs-session|%s|%d|%d", tenantID, authorizationID, keyVersion)
}
