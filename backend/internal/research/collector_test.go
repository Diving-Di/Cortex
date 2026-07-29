package research

import (
	"net"
	"testing"
)

func TestNormalizeURLRemovesTrackingAndRejectsUnsafeTargets(t *testing.T) {
	value, err := NormalizeURL("https://xiaohongshu.com/explore/abc/?utm_source=x&xsec_token=token#fragment")
	if err != nil {
		t.Fatal(err)
	}
	if value != "https://www.xiaohongshu.com/explore/abc?xsec_token=token" {
		t.Fatalf("unexpected normalized URL %q", value)
	}
	for _, unsafe := range []string{
		"http://www.xiaohongshu.com/explore/abc",
		"https://127.0.0.1/explore/abc",
		"https://example.com/explore/abc",
		"https://user:pass@www.xiaohongshu.com/explore/abc",
	} {
		if _, err := NormalizeURL(unsafe); err == nil {
			t.Fatalf("expected %q to be rejected", unsafe)
		}
	}
}

func TestIsPublicIPRejectsSpecialAndPrivateRanges(t *testing.T) {
	for _, value := range []string{
		"127.0.0.1", "10.0.0.1", "100.64.0.1", "169.254.169.254",
		"192.0.2.1", "198.18.0.1", "203.0.113.1", "::1", "fc00::1", "2001:db8::1",
	} {
		if isPublicIP(net.ParseIP(value)) {
			t.Errorf("%s must not be treated as public", value)
		}
	}
	for _, value := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"} {
		if !isPublicIP(net.ParseIP(value)) {
			t.Errorf("%s should be treated as public", value)
		}
	}
}

func TestValidateKeywords(t *testing.T) {
	values, err := ValidateKeywords([]string{" Agent ", "Agent", "RAG"}, 3)
	if err != nil || len(values) != 2 || values[0] != "Agent" {
		t.Fatalf("unexpected result %#v %v", values, err)
	}
	if _, err := ValidateKeywords(nil, 3); err == nil {
		t.Fatal("empty keywords must fail")
	}
}
