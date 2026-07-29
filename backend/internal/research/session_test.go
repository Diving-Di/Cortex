package research

import (
	"strings"
	"testing"
	"time"
)

func TestCookieHeaderScopesAndExpiresCookies(t *testing.T) {
	now := time.Unix(1_000, 0)
	future := float64(time.Now().Add(time.Hour).Unix())
	state := SessionState{Cookies: []SessionCookie{
		{Name: "web_session", Value: "secret", Domain: ".xiaohongshu.com", Expires: future},
		{Name: "expired", Value: "old", Domain: ".xiaohongshu.com", Expires: 900},
		{Name: "other", Value: "no", Domain: ".example.com", Expires: future},
	}}
	header := state.CookieHeader("www.xiaohongshu.com", now)
	if header != "web_session=secret" || strings.Contains(header, "old") {
		t.Fatalf("unexpected cookie header %q", header)
	}
	if !state.Authorized() {
		t.Fatal("web_session should mark state authorized")
	}
}

func TestSessionStateAuthorizedAcceptsVersionedXHSSessionCookie(t *testing.T) {
	state := SessionState{Cookies: []SessionCookie{{
		Name: "web_session_2", Value: "secret", Domain: ".www.xiaohongshu.com",
	}}}
	if !state.Authorized() {
		t.Fatal("versioned xiaohongshu session cookie should mark state authorized")
	}
}

func TestSessionStateAuthorizedRejectsUntrustedOrAnonymousCookies(t *testing.T) {
	for _, cookie := range []SessionCookie{
		{Name: "web_session", Value: "secret", Domain: ".example.com"},
		{Name: "a1", Value: "anonymous", Domain: ".xiaohongshu.com"},
		{Name: "web_session", Value: "", Domain: ".xiaohongshu.com"},
	} {
		if (SessionState{Cookies: []SessionCookie{cookie}}).Authorized() {
			t.Fatalf("cookie must not mark state authorized: %#v", cookie)
		}
	}
}
