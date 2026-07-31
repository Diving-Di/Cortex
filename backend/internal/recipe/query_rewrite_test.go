package recipe

import (
	"strings"
	"testing"
)

func TestRewriteQueryScopesSuggestedQuestionsToFeaturedRecipe(t *testing.T) {
	tests := []struct {
		question string
		intent   string
	}{
		{"需要哪些食材和用量？", "食材"},
		{"请完整说明制作步骤。", "步骤"},
		{"有哪些容易忽略的技巧？", "技巧"},
	}
	for _, test := range tests {
		t.Run(test.intent, func(t *testing.T) {
			got := RewriteQuery(test.question, "微波炉蛋糕")
			if !got.FeaturedOnly || !strings.Contains(got.Query, "微波炉蛋糕") ||
				!strings.Contains(got.Query, test.intent) {
				t.Fatalf("RewriteQuery() = %#v", got)
			}
		})
	}
}

func TestRewriteQueryAnchorsFreeQuestionWithoutRestrictingCorpus(t *testing.T) {
	got := RewriteQuery("没有黄油可以换成什么？", "微波炉蛋糕")
	if got.FeaturedOnly || !strings.Contains(got.Query, "微波炉蛋糕") ||
		!strings.Contains(got.Query, "没有黄油可以换成什么？") {
		t.Fatalf("RewriteQuery() = %#v", got)
	}
}
