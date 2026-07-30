package recipe

import (
	"errors"
	"path"
	"regexp"
	"strings"
)

var (
	headingPattern    = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)
	difficultyPattern = regexp.MustCompile(`(?m)^预估烹饪难度[：:]\s*(.+?)\s*$`)
	caloriesPattern   = regexp.MustCompile(`(?m)^预估卡路里[：:]\s*(.+?)\s*$`)
	listItemPattern   = regexp.MustCompile(`^\s*[-*]\s+(.+?)\s*$`)
)

// ParseMarkdown extracts stable recipe metadata while preserving the complete
// Markdown as the authoritative answer context.
func ParseMarkdown(sourcePath string, md []byte) (*RecipeDocument, error) {
	content := strings.ReplaceAll(string(md), "\r\n", "\n")
	if strings.TrimSpace(content) == "" {
		return nil, errors.New("empty markdown")
	}
	titleMatch := headingPattern.FindStringSubmatch(content)
	if len(titleMatch) != 2 {
		return nil, errors.New("missing level-one title")
	}

	kind := "dish"
	normalizedPath := strings.ReplaceAll(sourcePath, `\`, "/")
	if strings.Contains(normalizedPath, "/tips/") || strings.HasPrefix(normalizedPath, "tips/") {
		kind = "tip"
	}
	category := path.Base(path.Dir(normalizedPath))
	if category == "." || category == "/" {
		category = "uncategorized"
	}

	title := strings.TrimSpace(titleMatch[1])
	if kind == "dish" {
		title = strings.TrimSuffix(title, "的做法")
	}
	summary := firstParagraph(strings.TrimPrefix(content, titleMatch[0]))
	ingredientsSection := markdownSection(content, "## 必备原料和工具")
	ingredients := listItems(ingredientsSection)
	dietary := NormalizeDietaryTerms(ingredients)

	var difficulty, calories *string
	if match := difficultyPattern.FindStringSubmatch(content); len(match) == 2 {
		value := strings.TrimSpace(match[1])
		difficulty = &value
	}
	if match := caloriesPattern.FindStringSubmatch(content); len(match) == 2 {
		value := strings.TrimSpace(match[1])
		calories = &value
	}

	return &RecipeDocument{
		SourcePath:      normalizedPath,
		Kind:            kind,
		Category:        category,
		Title:           title,
		Summary:         summary,
		Ingredients:     ingredients,
		DietaryTerms:    dietary,
		Difficulty:      difficulty,
		CaloriesText:    calories,
		ContentMarkdown: content,
		IsActive:        true,
	}, nil
}

func firstParagraph(content string) string {
	for _, block := range strings.Split(content, "\n\n") {
		value := strings.TrimSpace(block)
		if value == "" || strings.HasPrefix(value, "#") {
			continue
		}
		return value
	}
	return ""
}

func markdownSection(content, heading string) string {
	start := strings.Index(content, heading)
	if start < 0 {
		return ""
	}
	section := content[start+len(heading):]
	if next := strings.Index(section, "\n## "); next >= 0 {
		section = section[:next]
	}
	return section
}

func listItems(section string) []string {
	result := []string{}
	for _, line := range strings.Split(section, "\n") {
		if match := listItemPattern.FindStringSubmatch(line); len(match) == 2 {
			result = append(result, strings.TrimSpace(match[1]))
		}
	}
	return result
}
