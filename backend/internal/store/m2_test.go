package store

import (
    "reflect"
    "testing"
)

func TestMemoryWordsRemovesQuestionPhrasesAndDeduplicates(t *testing.T) {
    got := memoryWords("请问回忆一下，本周项目发布做了什么？项目发布")
    want := []string{"项目发布", "项目", "目发", "发布"}
    if !reflect.DeepEqual(got, want) {
        t.Fatalf("memoryWords() = %#v, want %#v", got, want)
    }
}

func TestMemoryWordsLimitsResult(t *testing.T) {
    got := memoryWords("甲乙丙丁戊己庚辛壬癸")
    if len(got) != 8 {
        t.Fatalf("memoryWords() returned %d words, want 8", len(got))
    }
}

func TestSnippetUsesRunesAndNormalizesWhitespace(t *testing.T) {
    if got := snippet("  中文\n内容  测试 ", 5); got != "中文 内容" {
        t.Fatalf("snippet() = %q", got)
    }
}
