package recipe

import "strings"

// QueryRewrite is the retrieval query derived from a user's recipe question.
// FeaturedOnly is used for intents that must be answered from the featured
// recipe itself instead of semantically similar dishes.
type QueryRewrite struct {
	Query        string
	FeaturedOnly bool
}

// RewriteQuery anchors generic and follow-up questions to the featured recipe.
// This is deliberately deterministic so retrieval remains available when the
// optional generation model is unavailable.
func RewriteQuery(question, featuredTitle string) QueryRewrite {
	question = strings.TrimSpace(question)
	featuredTitle = strings.TrimSpace(featuredTitle)
	if featuredTitle == "" {
		return QueryRewrite{Query: question}
	}

	lower := strings.ToLower(question)
	switch {
	case containsAny(lower, "食材", "原料", "材料", "用量", "配料", "几克", "多少克"):
		return QueryRewrite{
			Query:        strings.Join([]string{featuredTitle, "食材 原料 配料 用量 配方", question}, " "),
			FeaturedOnly: true,
		}
	case containsAny(lower, "步骤", "怎么做", "如何做", "制作", "做法", "流程"):
		return QueryRewrite{
			Query:        strings.Join([]string{featuredTitle, "完整制作步骤 做法 流程", question}, " "),
			FeaturedOnly: true,
		}
	case containsAny(lower, "技巧", "注意", "容易忽略", "窍门", "失败", "避坑"):
		return QueryRewrite{
			Query:        strings.Join([]string{featuredTitle, "烹饪技巧 注意事项 避坑", question}, " "),
			FeaturedOnly: true,
		}
	default:
		return QueryRewrite{
			Query: strings.Join([]string{featuredTitle, question, "菜谱 烹饪 做法"}, " "),
		}
	}
}

func containsAny(value string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}
