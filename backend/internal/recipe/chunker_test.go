package recipe

import (
	"strings"
	"testing"
)

func TestChunkDocumentCreatesSectionParentsAndFocusedChildren(t *testing.T) {
	doc, err := ParseMarkdown("dishes/test.md", []byte(`# 测试菜的做法

简介。

## 必备原料和工具
- 土豆

## 操作
1. 切丝。
2. 翻炒。

## 附加内容
保持脆爽。

如果您遵循本指南的制作流程而发现有问题或可以改进的流程，请提出 Issue 或 Pull request 。`))
	if err != nil {
		t.Fatal(err)
	}
	parents := ChunkDocument(doc)
	if len(parents) != 4 {
		t.Fatalf("parents = %d", len(parents))
	}
	if parents[1].HeadingPath != "必备原料和工具" {
		t.Fatalf("heading = %q", parents[1].HeadingPath)
	}
	if !strings.Contains(parents[1].Children[0].EmbeddingText, "食材 原料 配料 用量 配方") {
		t.Fatal(parents[1].Children[0].EmbeddingText)
	}
	if strings.Contains(parents[3].Children[0].EmbeddingText, "Pull request") {
		t.Fatal("boilerplate was indexed")
	}
}

func TestChunkDocumentIsDeterministicAndSplitsLongSection(t *testing.T) {
	doc := &RecipeDocument{Title: "长文", Category: "tip", ContentMarkdown: "# 长文\n\n## 操作\n" + strings.Repeat("步骤内容。", 180)}
	one, two := ChunkDocument(doc), ChunkDocument(doc)
	if len(one) != 2 || len(one[1].Children) < 2 {
		t.Fatalf("unexpected chunks: %#v", one)
	}
	if one[1].Children[0].ContentHash != two[1].Children[0].ContentHash {
		t.Fatal("hash is not stable")
	}
	for _, child := range one[1].Children {
		if len([]rune(child.Content)) > 500 {
			t.Fatalf("child too long: %d", len([]rune(child.Content)))
		}
	}
}
