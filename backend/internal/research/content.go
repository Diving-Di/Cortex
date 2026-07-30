package research

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type ContentDiagnostics struct {
	ParseStrategy        string
	ContentCompleteness  int
	OCRContributionChars int
	FormatStatus         string
}

func PrepareContent(raw string, ocrTexts []string, parseStrategy string, imageCount int) (string, ContentDiagnostics) {
	raw = cleanExtractedText(raw)
	cleanOCR := make([]string, 0, len(ocrTexts))
	ocrChars := 0
	for _, value := range ocrTexts {
		value = cleanExtractedText(value)
		if value == "" {
			continue
		}
		cleanOCR = append(cleanOCR, value)
		ocrChars += utf8.RuneCountInString(value)
	}
	parts := make([]string, 0, 2)
	if raw != "" {
		parts = append(parts, raw)
	}
	if len(cleanOCR) > 0 {
		parts = append(parts, "## 图片提取文字\n\n"+strings.Join(cleanOCR, "\n\n"))
	}
	content := strings.Join(parts, "\n\n")
	if parseStrategy == "" {
		parseStrategy = "metadata"
	}
	rawChars := utf8.RuneCountInString(raw)
	score := min(60, rawChars/20)
	if parseStrategy == "browser_detail" {
		score += 20
	}
	if imageCount == 0 || ocrChars > 0 {
		score += 15
	}
	if rawChars+ocrChars >= 500 {
		score += 5
	}
	score = min(100, max(0, score))
	return content, ContentDiagnostics{
		ParseStrategy: parseStrategy, ContentCompleteness: score,
		OCRContributionChars: ocrChars, FormatStatus: "deterministic",
	}
}

func cleanExtractedText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimSpace(strings.Map(func(char rune) rune {
			if unicode.IsControl(char) && char != '\t' {
				return -1
			}
			return char
		}, line))
		if line == "" {
			if !blank && len(result) > 0 {
				result = append(result, "")
			}
			blank = true
			continue
		}
		result = append(result, line)
		blank = false
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}
