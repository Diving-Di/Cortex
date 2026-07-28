package research

import (
	"strings"
	"testing"
	"time"
)

func TestCookieHeaderScopesAndExpiresCookies(t *testing.T) {
	now := time.Unix(1_000, 0)
	state := SessionState{Cookies: []SessionCookie{
		{Name: "web_session", Value: "secret", Domain: ".xiaohongshu.com", Expires: 2_000},
		{Name: "expired", Value: "old", Domain: ".xiaohongshu.com", Expires: 900},
		{Name: "other", Value: "no", Domain: ".example.com", Expires: 2_000},
	}}
	header := state.CookieHeader("www.xiaohongshu.com", now)
	if header != "web_session=secret" || strings.Contains(header, "old") {
		t.Fatalf("unexpected cookie header %q", header)
	}
	if !state.Authorized() {
		t.Fatal("web_session should mark state authorized")
	}
}
