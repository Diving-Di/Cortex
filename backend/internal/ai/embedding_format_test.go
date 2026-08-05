package ai

import (
	"strings"
	"testing"
)

func TestFormatRerankDocumentIncludesRetrievalSignals(t *testing.T) {
	got := FormatRerankDocument(" 鱼香肉丝的做法 ", "upload", []string{"操作", "处理原料"}, " 腌制十五分钟 ")
	for _, want := range []string{"标题：鱼香肉丝的做法", "来源：upload", "章节：操作 / 处理原料", "内容：腌制十五分钟"} {
		if !strings.Contains(got, want) {
			t.Fatalf("FormatRerankDocument()=%q, missing %q", got, want)
		}
	}
}
