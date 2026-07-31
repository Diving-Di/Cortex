package store

import "testing"

func TestRecipeConversationTitleSummarizesFirstQuestion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"需要哪些食材和用量？", "需要哪些食材和用量"},
		{"请问一下 微波炉蛋糕怎么制作？还需要注意什么？", "微波炉蛋糕怎么制作"},
		{"   ", "菜谱问答"},
	}
	for _, test := range tests {
		if got := recipeConversationTitle(test.input); got != test.want {
			t.Fatalf("recipeConversationTitle(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestValidSourceScopeIncludesRecipe(t *testing.T) {
	if !ValidSourceScope("recipe") {
		t.Fatal("recipe source scope must be available through conversation APIs")
	}
}
