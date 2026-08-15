package store

import (
	"errors"
	"strings"
	"testing"

	"cortex/backend/internal/apierror"
)

func TestValidateTemplateInput(t *testing.T) {
	valid := TemplateInput{Title: "每日复盘", ContentMarkdown: "# 今日", Category: "reflection"}
	if err := validateTemplateInput(valid); err != nil {
		t.Fatalf("valid template: %v", err)
	}
	cases := []TemplateInput{{ContentMarkdown: "x", Category: "c"}, {Title: "x", Category: "c"}, {Title: "x", ContentMarkdown: "x", Category: ""}, {Title: "x", ContentMarkdown: strings.Repeat("a", 65537), Category: "c"}}
	for _, item := range cases {
		var target *apierror.Error
		if err := validateTemplateInput(item); !errors.As(err, &target) {
			t.Fatalf("expected validation error for %#v, got %v", item, err)
		}
	}
}

func TestTemplateAndNicknameSafety(t *testing.T) {
	if validatePublicNickname("Diary Listener 官方") {
		t.Fatal("reserved nickname accepted")
	}
	if validatePublicNickname("正常\u0000昵称") {
		t.Fatal("control character accepted")
	}
	if !validatePublicNickname("山间记录者") {
		t.Fatal("valid nickname rejected")
	}
	deep := strings.Repeat("[", 65) + "x"
	if validateTemplateInput(TemplateInput{Title: "x", ContentMarkdown: deep, Category: "c"}) == nil {
		t.Fatal("extreme nesting accepted")
	}
	for _, unsafe := range []string{
		"[click](javascript:alert(1))",
		"![tracking](https://example.test/pixel.png)",
		"<img src=https://example.test/pixel.png>",
		"key: sk-1234567890abcdef",
		"-----BEGIN PRIVATE KEY-----",
	} {
		if validateTemplateInput(TemplateInput{Title: "x", ContentMarkdown: unsafe, Category: "c"}) == nil {
			t.Fatalf("unsafe markdown accepted: %q", unsafe)
		}
	}
}
