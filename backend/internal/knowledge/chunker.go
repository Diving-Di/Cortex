package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
)

type ChunkOptions struct {
	ParentTargetTokens int
	ParentMaxTokens    int
	ChildTargetTokens  int
	ChildMaxTokens     int
	ChildOverlapTokens int
}

func DefaultChunkOptions() ChunkOptions {
	return ChunkOptions{
		ParentTargetTokens: 1800,
		ParentMaxTokens:    2500,
		ChildTargetTokens:  350,
		ChildMaxTokens:     500,
		ChildOverlapTokens: 50,
	}
}

func BuildParentChildChunks(document Document, options ChunkOptions) []ParentChunk {
	if options.ParentTargetTokens <= 0 || options.ParentMaxTokens < options.ParentTargetTokens ||
		options.ChildTargetTokens <= 0 || options.ChildMaxTokens < options.ChildTargetTokens ||
		options.ChildOverlapTokens < 0 || options.ChildOverlapTokens >= options.ChildTargetTokens {
		options = DefaultChunkOptions()
	}
	groups := parentGroups(document.Blocks, options)
	result := make([]ParentChunk, 0, len(groups))
	for index, group := range groups {
		parent := ParentChunk{
			Index: index, Content: renderBlocks(group), HeadingPath: commonHeading(group),
			PageFrom: firstPage(group), PageTo: lastPage(group),
		}
		parent.TokenCount = estimateTokens(parent.Content)
		parent.Children = childChunks(document.Title, parent, group, options)
		result = append(result, parent)
	}
	return result
}

func parentGroups(blocks []Block, options ChunkOptions) [][]Block {
	var result [][]Block
	var current []Block
	currentTokens := 0
	currentHeading := ""
	flush := func() {
		if len(current) > 0 {
			result = append(result, current)
		}
		current = nil
		currentTokens = 0
	}
	for _, block := range blocks {
		tokens := estimateTokens(block.Text)
		heading := strings.Join(block.HeadingPath, "\x1f")
		structureBreak := len(current) > 0 && block.Kind == BlockHeading &&
			currentTokens >= options.ParentTargetTokens/3
		headingBreak := len(current) > 0 && currentHeading != "" && heading != "" &&
			heading != currentHeading && currentTokens >= options.ParentTargetTokens/2
		if structureBreak || headingBreak || (len(current) > 0 && currentTokens+tokens > options.ParentMaxTokens) {
			flush()
		}
		if tokens > options.ParentMaxTokens {
			if block.Kind == BlockTable {
				for _, part := range splitMarkdownTable(block.Text, options.ParentMaxTokens) {
					cloned := block
					cloned.Text = part
					if len(current) > 0 {
						flush()
					}
					result = append(result, []Block{cloned})
				}
				continue
			}
			for _, part := range splitText(block.Text, options.ParentMaxTokens, 0) {
				cloned := block
				cloned.Text = part
				if len(current) > 0 {
					flush()
				}
				result = append(result, []Block{cloned})
			}
			continue
		}
		current = append(current, block)
		currentTokens += tokens
		if heading != "" {
			currentHeading = heading
		}
		if currentTokens >= options.ParentTargetTokens && block.Kind != BlockHeading {
			flush()
		}
	}
	flush()
	return result
}

func splitMarkdownTable(value string, maxTokens int) []string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	if len(lines) < 3 || maxTokens <= 0 {
		return []string{value}
	}
	header := lines[:2]
	var result []string
	current := append([]string(nil), header...)
	for _, row := range lines[2:] {
		candidate := strings.Join(append(current, row), "\n")
		if len(current) > 2 && estimateTokens(candidate) > maxTokens {
			result = append(result, strings.Join(current, "\n"))
			current = append(append([]string(nil), header...), row)
		} else {
			current = append(current, row)
		}
	}
	if len(current) > 2 {
		result = append(result, strings.Join(current, "\n"))
	}
	if len(result) == 0 {
		return []string{value}
	}
	return result
}

func childChunks(title string, parent ParentChunk, blocks []Block, options ChunkOptions) []ChildChunk {
	var units []Block
	for _, block := range blocks {
		if estimateTokens(block.Text) <= options.ChildMaxTokens {
			units = append(units, block)
			continue
		}
		for _, part := range splitText(block.Text, options.ChildTargetTokens, options.ChildOverlapTokens) {
			cloned := block
			cloned.Text = part
			units = append(units, cloned)
		}
	}
	var result []ChildChunk
	var current []Block
	tokens := 0
	flush := func() {
		if len(current) == 0 {
			return
		}
		content := renderBlocks(current)
		heading := commonHeading(current)
		prefix := title
		if len(heading) > 0 {
			prefix += " > " + strings.Join(heading, " > ")
		}
		result = append(result, ChildChunk{
			Index: len(result), Content: content, EmbeddingText: prefix + "\n" + content,
			HeadingPath: heading, PageFrom: firstPage(current), PageTo: lastPage(current),
			TokenCount: estimateTokens(content),
		})
		current = nil
		tokens = 0
	}
	for _, unit := range units {
		unitTokens := estimateTokens(unit.Text)
		if len(current) > 0 && tokens+unitTokens > options.ChildMaxTokens {
			flush()
		}
		current = append(current, unit)
		tokens += unitTokens
		if tokens >= options.ChildTargetTokens {
			flush()
		}
	}
	flush()
	if len(result) == 0 && parent.Content != "" {
		result = append(result, ChildChunk{
			Index: 0, Content: parent.Content, EmbeddingText: title + "\n" + parent.Content,
			HeadingPath: parent.HeadingPath, PageFrom: parent.PageFrom, PageTo: parent.PageTo,
			TokenCount: parent.TokenCount,
		})
	}
	return result
}

func splitText(value string, target, overlap int) []string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) == 0 {
		return nil
	}
	var result []string
	for start := 0; start < len(runes); {
		end := tokenBoundedEnd(runes, start, target)
		if end < len(runes) {
			lowerBound := start + (end-start)/2
			for index := end; index > lowerBound; index-- {
				if strings.ContainsRune("。！？.!?\n；;", runes[index-1]) {
					end = index
					break
				}
			}
		}
		result = append(result, strings.TrimSpace(string(runes[start:end])))
		if end >= len(runes) {
			break
		}
		next := tokenOverlapStart(runes, start, end, overlap)
		if next <= start {
			next = end
		}
		start = next
	}
	return result
}

func tokenBoundedEnd(runes []rune, start, target int) int {
	if target <= 0 {
		return min(len(runes), start+1)
	}
	low, high := start+1, len(runes)
	best := low
	for low <= high {
		middle := low + (high-low)/2
		if estimateTokens(string(runes[start:middle])) <= target {
			best = middle
			low = middle + 1
		} else {
			high = middle - 1
		}
	}
	return best
}

func tokenOverlapStart(runes []rune, start, end, overlap int) int {
	if overlap <= 0 {
		return end
	}
	low, high := start+1, end
	best := end
	for low <= high {
		middle := low + (high-low)/2
		if estimateTokens(string(runes[middle:end])) <= overlap {
			best = middle
			high = middle - 1
		} else {
			low = middle + 1
		}
	}
	return best
}

func renderBlocks(blocks []Block) string {
	var values []string
	for _, block := range blocks {
		value := strings.TrimSpace(block.Text)
		if value == "" {
			continue
		}
		if block.Kind == BlockHeading && !strings.HasPrefix(value, "#") {
			value = "## " + value
		}
		values = append(values, value)
	}
	return strings.Join(values, "\n\n")
}

func commonHeading(blocks []Block) []string {
	if len(blocks) == 0 {
		return nil
	}
	result := append([]string(nil), blocks[0].HeadingPath...)
	for _, block := range blocks[1:] {
		limit := min(len(result), len(block.HeadingPath))
		index := 0
		for index < limit && result[index] == block.HeadingPath[index] {
			index++
		}
		result = result[:index]
	}
	return result
}

func firstPage(blocks []Block) int {
	for _, block := range blocks {
		if block.PageFrom > 0 {
			return block.PageFrom
		}
	}
	return 0
}

func lastPage(blocks []Block) int {
	for index := len(blocks) - 1; index >= 0; index-- {
		if blocks[index].PageTo > 0 {
			return blocks[index].PageTo
		}
	}
	return 0
}

func estimateTokens(value string) int {
	var tokens int
	inASCIIWord := false
	for _, item := range value {
		switch {
		case unicode.In(item, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul):
			tokens++
			inASCIIWord = false
		case unicode.IsLetter(item) || unicode.IsDigit(item):
			if !inASCIIWord {
				tokens++
				inASCIIWord = true
			}
		default:
			inASCIIWord = false
			if !unicode.IsSpace(item) {
				tokens++
			}
		}
	}
	return max(1, tokens)
}

// EstimateTokens returns the deterministic conservative token estimate used by
// chunking and request-budget enforcement.
func EstimateTokens(value string) int {
	return estimateTokens(value)
}

func ContentHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

// SearchLexicalText adds deterministic CJK bigrams while preserving the
// original text, giving PostgreSQL's simple dictionary useful Chinese tokens.
func SearchLexicalText(value string) string {
	var tokens []string
	var cjk []rune
	flush := func() {
		for index := 0; index+1 < len(cjk); index++ {
			tokens = append(tokens, string(cjk[index:index+2]))
		}
		cjk = nil
	}
	for _, item := range strings.ToLower(value) {
		if unicode.In(item, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
			cjk = append(cjk, item)
			continue
		}
		flush()
	}
	flush()
	return value + "\n" + strings.Join(tokens, " ")
}
