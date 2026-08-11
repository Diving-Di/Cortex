package knowledge

import (
	"strings"
	"testing"
)

func TestKeywordTextChineseAndMixedContent(t *testing.T) {
	got := KeywordText(" ＰｏｓｔｇｒｅＳＱＬ 16：中文检索，P95=175ms ")
	want := "postgresql 16 中文 文检 检索 p95 175ms"
	if got != want {
		t.Fatalf("KeywordText()=%q, want %q", got, want)
	}
}

func TestKeywordTextKeepsSingleHanAndDropsPunctuation(t *testing.T) {
	if got := KeywordText("甲，A+B！"); got != "甲 a b" {
		t.Fatalf("KeywordText()=%q", got)
	}
}

func TestKeywordQueryTextDeduplicatesAndBoundsTokens(t *testing.T) {
	if got := KeywordQueryText("中文中文中文 PostgreSQL PostgreSQL", 3); got != "中文 文中 postgresql" {
		t.Fatalf("KeywordQueryText()=%q", got)
	}
	if got := KeywordQueryText("中文", 0); got != "" {
		t.Fatalf("zero-limit query=%q", got)
	}
}

func TestChunkSkipsHeadingOnlyParent(t *testing.T) {
	markdown := "# 菜谱\n简介\n\n## 操作\n\n### 处理原料\n切丝并腌制。\n"
	parents := Chunk("菜谱", "upload", markdown)
	if len(parents) != 2 {
		t.Fatalf("Chunk() returned %d parents, want 2: %#v", len(parents), parents)
	}
	for _, parent := range parents {
		if len(parent.Heading) == 2 && parent.Heading[1] == "操作" {
			t.Fatalf("heading-only parent was indexed: %#v", parent)
		}
	}
	if got := parents[1].Heading; len(got) != 3 || got[2] != "处理原料" {
		t.Fatalf("child heading path=%v", got)
	}
}

func TestChunkKeepsHeadingWithBody(t *testing.T) {
	parents := Chunk("菜谱", "upload", "## 操作\n第一步。")
	if len(parents) != 1 || len(parents[0].Children) == 0 {
		t.Fatalf("informative heading was dropped: %#v", parents)
	}
}

func TestChunkStoresKeywordTextAndVersionsHash(t *testing.T) {
	chunks := Chunk("数据库笔记", "note", "PostgreSQL支持事务隔离")
	if len(chunks) != 1 || len(chunks[0].Children) != 1 {
		t.Fatalf("chunks=%#v", chunks)
	}
	child := chunks[0].Children[0]
	for _, token := range []string{"数据", "据库", "事务", "隔离", "postgresql"} {
		if !strings.Contains(child.KeywordText, token) {
			t.Fatalf("keyword_text=%q missing %q", child.KeywordText, token)
		}
	}
	if child.Hash == hash("child-v1\n"+child.EmbeddingText) {
		t.Fatal("child hash did not change with keyword index version")
	}
}
