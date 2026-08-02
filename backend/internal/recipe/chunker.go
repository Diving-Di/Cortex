package recipe

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"diary-listener/backend/internal/store"
)

const RecipeIndexVersion = 2

var markdownHeadingLine = regexp.MustCompile(`^(#{2,3})\s+(.+?)\s*$`)

// ChunkDocument creates section parents and focused retrieval children. The
// output is deterministic and does not depend on an AI model.
func ChunkDocument(doc *RecipeDocument) []store.RecipeParentChunk {
	sections := splitMarkdownSections(doc.ContentMarkdown)
	parents := make([]store.RecipeParentChunk, 0, len(sections))
	childIndex := 0
	for parentIndex, section := range sections {
		pieces := splitSection(section.content, 500)
		children := make([]store.RecipeChildChunk, 0, len(pieces))
		for _, piece := range pieces {
			clean := cleanEmbeddingContent(piece)
			if clean == "" {
				continue
			}
			embeddingText := buildEmbeddingText(doc, section.heading, clean)
			hashInput := fmt.Sprintf("v%d\n%s\n%s", RecipeIndexVersion, section.heading, piece)
			sum := sha256.Sum256([]byte(hashInput))
			children = append(children, store.RecipeChildChunk{
				ChildIndex: childIndex, HeadingPath: section.heading, Content: piece,
				EmbeddingText: embeddingText, ContentHash: hex.EncodeToString(sum[:]),
				TokenCount: estimateTokens(piece),
			})
			childIndex++
		}
		if len(children) == 0 {
			continue
		}
		parents = append(parents, store.RecipeParentChunk{ParentIndex: parentIndex,
			HeadingPath: section.heading, Content: section.content,
			TokenCount: estimateTokens(section.content), Children: children})
	}
	return parents
}

type markdownSectionValue struct {
	heading string
	content string
}

func splitMarkdownSections(markdown string) []markdownSectionValue {
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
	lines := strings.Split(markdown, "\n")
	var sections []markdownSectionValue
	heading := "简介与基础信息"
	var body []string
	flush := func() {
		content := strings.TrimSpace(strings.Join(body, "\n"))
		if content != "" {
			sections = append(sections, markdownSectionValue{heading: heading, content: content})
		}
		body = nil
	}
	for _, line := range lines {
		match := markdownHeadingLine.FindStringSubmatch(line)
		if len(match) == 3 && match[1] == "##" {
			flush()
			heading = strings.TrimSpace(match[2])
			body = append(body, line)
			continue
		}
		body = append(body, line)
	}
	flush()
	return sections
}

func splitSection(content string, maxRunes int) []string {
	if len([]rune(content)) <= maxRunes {
		return []string{content}
	}
	blocks := strings.Split(content, "\n")
	var result []string
	var current strings.Builder
	flush := func() {
		if value := strings.TrimSpace(current.String()); value != "" {
			result = append(result, value)
		}
		current.Reset()
	}
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		if current.Len() > 0 && len([]rune(current.String()+"\n"+block)) > maxRunes {
			flush()
		}
		if len([]rune(block)) > maxRunes {
			runes := []rune(block)
			for len(runes) > 0 {
				prefix := ""
				if current.Len() > 0 {
					prefix = current.String() + "\n"
				}
				capacity := maxRunes - len([]rune(prefix))
				if capacity <= 0 {
					flush()
					continue
				}
				take := min(capacity, len(runes))
				current.WriteString(string(runes[:take]))
				runes = runes[take:]
				if len(runes) > 0 {
					flush()
				}
			}
			continue
		}
		if current.Len() > 0 {
			current.WriteByte('\n')
		}
		current.WriteString(block)
	}
	flush()
	return result
}

func cleanEmbeddingContent(content string) string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "![") || strings.Contains(trimmed, "提出 Issue 或 Pull request") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func buildEmbeddingText(doc *RecipeDocument, heading, content string) string {
	intent := "特点 风味 难度 热量"
	lower := strings.ToLower(heading)
	switch {
	case strings.Contains(lower, "原料"), strings.Contains(lower, "食材"), strings.Contains(lower, "计算"):
		intent = "食材 原料 配料 用量 配方"
	case strings.Contains(lower, "操作"), strings.Contains(lower, "做法"), strings.Contains(lower, "制作"):
		intent = "步骤 做法 流程 烹饪"
	case strings.Contains(lower, "附加"), strings.Contains(lower, "注意"), strings.Contains(lower, "技巧"):
		intent = "技巧 注意 避坑 口感 保存"
	}
	return fmt.Sprintf("标题：%s\n分类：%s\n章节：%s\n检索意图：%s\n食材：%s\n内容：%s",
		doc.Title, doc.Category, heading, intent, strings.Join(doc.Ingredients, " "), content)
}

func estimateTokens(content string) int { return max(1, len([]rune(content))/2) }
