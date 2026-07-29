package research

import (
	"strings"
	"time"
)

type SessionState struct {
	Cookies []SessionCookie `json:"cookies"`
}

type SessionCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	Secure   bool    `json:"secure"`
	HTTPOnly bool    `json:"http_only"`
	SameSite string  `json:"same_site"`
}

func (s SessionState) CookieHeader(host string, now time.Time) string {
	host = strings.ToLower(host)
	var values []string
	for _, cookie := range s.Cookies {
		domain := strings.TrimPrefix(strings.ToLower(cookie.Domain), ".")
		if domain == "" || (host != domain && !strings.HasSuffix(host, "."+domain)) {
			continue
		}
		if cookie.Expires > 0 && time.Unix(int64(cookie.Expires), 0).Before(now) {
			continue
		}
		if strings.TrimSpace(cookie.Name) == "" || strings.ContainsAny(cookie.Name, "\r\n;") ||
			strings.ContainsAny(cookie.Value, "\r\n") {
			continue
		}
		values = append(values, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(values, "; ")
}

func (s SessionState) Authorized() bool {
	for _, cookie := range s.Cookies {
		name := strings.ToLower(strings.TrimSpace(cookie.Name))
		domain := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(cookie.Domain)), ".")
		isXHSDomain := domain == "xiaohongshu.com" || strings.HasSuffix(domain, ".xiaohongshu.com")
		isSessionCookie := name == "web_session" || strings.HasPrefix(name, "web_session_")
		notExpired := cookie.Expires <= 0 || time.Unix(int64(cookie.Expires), 0).After(time.Now())
		if isXHSDomain && isSessionCookie && notExpired && strings.TrimSpace(cookie.Value) != "" {
			return true
		}
	}
	return false
}
