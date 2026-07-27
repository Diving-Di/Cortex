package knowledge

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestBuildParentChildChunksPreservesStructure(t *testing.T) {
	document := Document{
		Title: "中英成长资料",
		Blocks: []Block{
			{Kind: BlockHeading, Text: "第一章", HeadingPath: []string{"第一章"}, PageFrom: 1, PageTo: 1},
			{Kind: BlockParagraph, Text: strings.Repeat("刻意练习需要反馈。", 120), HeadingPath: []string{"第一章"}, PageFrom: 1, PageTo: 2},
			{Kind: BlockHeading, Text: "Second Chapter", HeadingPath: []string{"Second Chapter"}, PageFrom: 3, PageTo: 3},
			{Kind: BlockParagraph, Text: strings.Repeat("Reflection needs evidence. ", 100), HeadingPath: []string{"Second Chapter"}, PageFrom: 3, PageTo: 4},
		},
	}
	options := DefaultChunkOptions()
	options.ParentTargetTokens = 300
	options.ParentMaxTokens = 450
	options.ChildTargetTokens = 80
	options.ChildMaxTokens = 120
	options.ChildOverlapTokens = 10
	parents := BuildParentChildChunks(document, options)
	if len(parents) < 2 {
		t.Fatalf("expected multiple parents, got %d", len(parents))
	}
	for _, parent := range parents {
		if parent.Content == "" || len(parent.Children) == 0 {
			t.Fatalf("empty parent or children: %#v", parent)
		}
		for _, child := range parent.Children {
			if child.Content == "" || !strings.Contains(child.EmbeddingText, document.Title) {
				t.Fatalf("invalid child: %#v", child)
			}
		}
	}
}

func TestSplitMarkdownTableRepeatsHeader(t *testing.T) {
	table := "| 名称 | 数值 |\n| --- | --- |\n"
	for index := 0; index < 30; index++ {
		table += fmt.Sprintf("| 项目 %d | 一段用于触发切分的较长内容 %d |\n", index, index)
	}
	parts := splitMarkdownTable(table, 35)
	if len(parts) < 2 {
		t.Fatalf("expected table split, got %d part", len(parts))
	}
	for _, part := range parts {
		if !strings.HasPrefix(part, "| 名称 | 数值 |\n| --- | --- |") {
			t.Fatalf("header was not repeated: %q", part)
		}
	}
}

func TestExtractTXTBuildsBlocks(t *testing.T) {
	path := t.TempDir() + "/sample.txt"
	content := "# 目标\n\n每天复盘。\n\n- 保留证据\n- 调整计划"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	document, err := Extract(context.Background(), path, ".txt", "sample", ExtractLimits{
		MaxPages: 10, MaxCharacters: 10000, TimeoutSecs: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Blocks) != 3 || document.Language != "zh" {
		t.Fatalf("unexpected document: %#v", document)
	}
}

func TestSplitTextHonorsTokenLimitForChineseAndTerminatesWithOverlap(t *testing.T) {
	value := strings.Repeat("成长需要证据和持续反馈。", 200)
	parts := splitText(value, 80, 12)
	if len(parts) < 2 {
		t.Fatalf("expected multiple parts, got %d", len(parts))
	}
	for index, part := range parts {
		if tokens := estimateTokens(part); tokens > 80 {
			t.Fatalf("part %d has %d tokens", index, tokens)
		}
		if strings.TrimSpace(part) == "" {
			t.Fatalf("part %d is empty", index)
		}
	}
	if len(parts) > len([]rune(value)) {
		t.Fatalf("overlap did not make forward progress: %d parts", len(parts))
	}
}

func TestBuildParentChildChunksKeepsConfiguredMaximums(t *testing.T) {
	document := Document{
		Title: "边界测试",
		Blocks: []Block{{
			Kind: BlockParagraph, Text: strings.Repeat("复盘", 800),
			HeadingPath: []string{"测试"},
		}},
	}
	options := ChunkOptions{
		ParentTargetTokens: 120, ParentMaxTokens: 160,
		ChildTargetTokens: 40, ChildMaxTokens: 60, ChildOverlapTokens: 6,
	}
	for _, parent := range BuildParentChildChunks(document, options) {
		if parent.TokenCount > options.ParentMaxTokens {
			t.Fatalf("parent has %d tokens", parent.TokenCount)
		}
		for _, child := range parent.Children {
			if child.TokenCount > options.ChildMaxTokens {
				t.Fatalf("child has %d tokens", child.TokenCount)
			}
		}
	}
}
