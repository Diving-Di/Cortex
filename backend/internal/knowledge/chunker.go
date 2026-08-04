package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

type ParentChunk struct {
	Heading       []string
	Content, Hash string
	Children      []ChildChunk
}
type ChildChunk struct{ Content, EmbeddingText, Hash string }

var headingPattern = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*$`)

func Chunk(title, sourceType, markdown string) []ParentChunk {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	path := []string{}
	body := []string{}
	var result []ParentChunk
	flush := func() {
		content := strings.TrimSpace(strings.Join(body, "\n"))
		if content == "" {
			body = nil
			return
		}
		p := ParentChunk{Heading: append([]string(nil), path...), Content: content}
		p.Hash = hash("parent-v1\n" + strings.Join(path, "/") + "\n" + content)
		for _, piece := range split(content, 500) {
			clean := strings.TrimSpace(piece)
			if clean == "" {
				continue
			}
			embedding := fmt.Sprintf("标题：%s\n来源：%s\n章节：%s\n内容：%s", title, sourceType, strings.Join(path, " / "), clean)
			p.Children = append(p.Children, ChildChunk{Content: clean, EmbeddingText: embedding, Hash: hash("child-v1\n" + embedding)})
		}
		if len(p.Children) > 0 {
			result = append(result, p)
		}
		body = nil
	}
	for _, line := range lines {
		match := headingPattern.FindStringSubmatch(line)
		if len(match) == 3 {
			flush()
			level := len(match[1])
			if level <= len(path) {
				path = path[:level-1]
			}
			for len(path) < level-1 {
				path = append(path, "")
			}
			path = append(path, strings.TrimSpace(match[2]))
			body = append(body, line)
		} else {
			body = append(body, line)
		}
	}
	flush()
	return result
}
func split(content string, maxRunes int) []string {
	r := []rune(content)
	var out []string
	for len(r) > 0 {
		n := min(maxRunes, len(r))
		if n < len(r) {
			for i := n; i > maxRunes/2; i-- {
				if r[i-1] == '\n' {
					n = i
					break
				}
			}
		}
		out = append(out, string(r[:n]))
		r = r[n:]
	}
	return out
}
func hash(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }
