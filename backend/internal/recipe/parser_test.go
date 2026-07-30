package recipe

import "testing"

func TestParseMarkdownDish(t *testing.T) {
	doc, err := ParseMarkdown(`dishes\aquatic\红烧鱼.md`, []byte(`# 红烧鱼的做法

一道家常菜。

预估烹饪难度：★★★★

预估卡路里：570 大卡

## 必备原料和工具

- 鱼
- 姜、蒜

## 操作

1. 煎鱼。
`))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Title != "红烧鱼" || doc.Kind != "dish" || doc.Category != "aquatic" {
		t.Fatalf("unexpected metadata: %#v", doc)
	}
	if doc.SourcePath != "dishes/aquatic/红烧鱼.md" {
		t.Fatalf("source path was not normalized: %q", doc.SourcePath)
	}
	if len(doc.Ingredients) != 2 || doc.Difficulty == nil || doc.CaloriesText == nil {
		t.Fatalf("missing extracted fields: %#v", doc)
	}
}

func TestParseMarkdownRejectsMissingTitle(t *testing.T) {
	if _, err := ParseMarkdown("bad.md", []byte("no title")); err == nil {
		t.Fatal("expected missing title error")
	}
}
