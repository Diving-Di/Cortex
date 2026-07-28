package research

import (
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

func TestValidateKeywords(t *testing.T) {
	values, err := ValidateKeywords([]string{" Agent ", "Agent", "RAG"}, 3)
	if err != nil || len(values) != 2 || values[0] != "Agent" {
		t.Fatalf("unexpected result %#v %v", values, err)
	}
	if _, err := ValidateKeywords(nil, 3); err == nil {
		t.Fatal("empty keywords must fail")
	}
}
