package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

type ParentChunk struct {
	Heading       []string
	Content, Hash string
	Children      []ChildChunk
}
type ChildChunk struct{ Content, EmbeddingText, KeywordText, Hash string }

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
		lines := strings.Split(content, "\n")
		if len(lines) == 1 && headingPattern.MatchString(strings.TrimSpace(lines[0])) {
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
			keywords := KeywordText(embedding)
			p.Children = append(p.Children, ChildChunk{Content: clean, EmbeddingText: embedding, KeywordText: keywords, Hash: hash("child-v2\n" + embedding + "\n" + keywords)})
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

// KeywordText returns deterministic lexemes for PostgreSQL's simple FTS
// configuration. Consecutive Han text is represented by overlapping 2-grams;
// Latin letters and numbers remain whole normalized words.
func KeywordText(value string) string {
	value = strings.ToLower(norm.NFKC.String(value))
	var tokens []string
	var run []rune
	var kind int
	flush := func() {
		if len(run) == 0 {
			return
		}
		if kind == 1 && len(run) > 1 {
			for i := 0; i+1 < len(run); i++ {
				tokens = append(tokens, string(run[i:i+2]))
			}
		} else {
			tokens = append(tokens, string(run))
		}
		run = run[:0]
	}
	for _, r := range value {
		nextKind := 0
		switch {
		case unicode.Is(unicode.Han, r):
			nextKind = 1
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			nextKind = 2
		}
		if nextKind == 0 {
			flush()
			kind = 0
			continue
		}
		if kind != 0 && kind != nextKind {
			flush()
		}
		kind = nextKind
		run = append(run, r)
	}
	flush()
	return strings.Join(tokens, " ")
}

// KeywordQueryText bounds and deduplicates query lexemes so a long user
// question cannot create an excessively large PostgreSQL tsquery.
func KeywordQueryText(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	seen := make(map[string]bool, limit)
	result := make([]string, 0, limit)
	for _, token := range strings.Fields(KeywordText(value)) {
		if seen[token] {
			continue
		}
		seen[token] = true
		result = append(result, token)
		if len(result) == limit {
			break
		}
	}
	return strings.Join(result, " ")
}
