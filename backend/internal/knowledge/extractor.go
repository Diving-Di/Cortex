package knowledge

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var (
	ErrEncrypted   = errors.New("document encrypted")
	ErrOCRRequired = errors.New("document OCR required")
	ErrParseLimit  = errors.New("document parse limit exceeded")
)

func Extract(ctx context.Context, path, extension, title string, limits ExtractLimits) (Document, error) {
	switch strings.ToLower(extension) {
	case ".txt":
		return extractTXT(path, title, limits)
	case ".md":
		return extractMarkdown(path, title, limits)
	case ".docx":
		return extractDOCX(path, title, limits)
	case ".pdf":
		return extractPDF(ctx, path, title, limits)
	default:
		return Document{}, fmt.Errorf("unsupported extension %q", extension)
	}
}

func extractMarkdown(path, title string, limits ExtractLimits) (Document, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Document{}, err
	}
	text := strings.TrimPrefix(string(content), "\ufeff")
	if len([]rune(text)) > limits.MaxCharacters {
		return Document{}, ErrParseLimit
	}

	var (
		blocks      []Block
		headingPath []string
		order       int
	)
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		kind := BlockParagraph
		value := line
		if level, heading := markdownHeading(line); level > 0 {
			kind = BlockHeading
			value = heading
			for len(headingPath) >= level {
				headingPath = headingPath[:len(headingPath)-1]
			}
			headingPath = append(headingPath, value)
		} else if isListLine(line) {
			kind = BlockList
		}
		blocks = append(blocks, Block{
			Kind: kind, Text: value, HeadingPath: append([]string(nil), headingPath...),
			PageFrom: 1, PageTo: 1, Order: order,
		})
		order++
	}
	return Document{
		Title: title, PageCount: 1, Language: detectLanguage(text),
		Blocks: blocks, Characters: len([]rune(text)),
	}, nil
}

func markdownHeading(line string) (int, string) {
	level := 0
	for level < len(line) && level < 6 && line[level] == '#' {
		level++
	}
	if level == 0 || level >= len(line) || line[level] != ' ' {
		return 0, ""
	}
	value := strings.TrimSpace(line[level:])
	if value == "" {
		return 0, ""
	}
	return level, value
}

func extractTXT(path, title string, limits ExtractLimits) (Document, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Document{}, err
	}
	text := strings.TrimPrefix(string(content), "\ufeff")
	if len([]rune(text)) > limits.MaxCharacters {
		return Document{}, ErrParseLimit
	}
	blocks := textBlocks(text, 1)
	return Document{
		Title: title, PageCount: 1, Language: detectLanguage(text),
		Blocks: blocks, Characters: len([]rune(text)),
	}, nil
}

type docxParagraph struct {
	Style string
	Text  string
	List  bool
}

func extractDOCX(path, title string, limits ExtractLimits) (Document, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return Document{}, err
	}
	defer archive.Close()
	var documentFile *zip.File
	for _, item := range archive.File {
		if item.Name == "word/document.xml" {
			documentFile = item
			break
		}
	}
	if documentFile == nil {
		return Document{}, errors.New("word/document.xml missing")
	}
	maxXMLBytes := int64(limits.MaxCharacters)*8 + 1
	if limits.MaxCharacters <= 0 || documentFile.UncompressedSize64 > uint64(maxXMLBytes) {
		return Document{}, ErrParseLimit
	}
	stream, err := documentFile.Open()
	if err != nil {
		return Document{}, err
	}
	defer stream.Close()
	limited := io.LimitReader(stream, maxXMLBytes)
	decoder := xml.NewDecoder(limited)
	decoder.Strict = true
	var (
		blocks      []Block
		headingPath []string
		paragraph   *docxParagraph
		inText      bool
		textBuffer  strings.Builder
		tableDepth  int
		tableRows   [][]string
		currentRow  []string
		currentCell strings.Builder
		page        = 1
		order       int
		characters  int
	)
	for {
		token, tokenErr := decoder.Token()
		if errors.Is(tokenErr, io.EOF) {
			break
		}
		if tokenErr != nil {
			return Document{}, tokenErr
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "p":
				if tableDepth == 0 {
					paragraph = &docxParagraph{}
					textBuffer.Reset()
				}
			case "pStyle":
				if paragraph != nil {
					for _, attribute := range value.Attr {
						if attribute.Name.Local == "val" {
							paragraph.Style = attribute.Value
						}
					}
				}
			case "numPr":
				if paragraph != nil {
					paragraph.List = true
				}
			case "t":
				inText = true
			case "tab":
				if tableDepth > 0 {
					currentCell.WriteByte('\t')
				} else {
					textBuffer.WriteByte('\t')
				}
			case "br":
				if tableDepth > 0 {
					currentCell.WriteByte('\n')
				} else {
					textBuffer.WriteByte('\n')
				}
			case "lastRenderedPageBreak":
				page++
				if page > limits.MaxPages {
					return Document{}, ErrParseLimit
				}
			case "tbl":
				tableDepth++
				if tableDepth == 1 {
					tableRows = nil
				}
			case "tr":
				if tableDepth > 0 {
					currentRow = nil
				}
			case "tc":
				if tableDepth > 0 {
					currentCell.Reset()
				}
			}
		case xml.CharData:
			if inText {
				if tableDepth > 0 {
					currentCell.Write([]byte(value))
				} else {
					textBuffer.Write([]byte(value))
				}
			}
		case xml.EndElement:
			switch value.Name.Local {
			case "t":
				inText = false
			case "p":
				if paragraph != nil && tableDepth == 0 {
					paragraph.Text = strings.TrimSpace(textBuffer.String())
					if paragraph.Text != "" {
						kind := BlockParagraph
						level := headingLevel(paragraph.Style)
						if level > 0 {
							kind = BlockHeading
							for len(headingPath) >= level {
								headingPath = headingPath[:len(headingPath)-1]
							}
							headingPath = append(headingPath, paragraph.Text)
						} else if paragraph.List {
							kind = BlockList
						}
						characters += len([]rune(paragraph.Text))
						if characters > limits.MaxCharacters {
							return Document{}, ErrParseLimit
						}
						blocks = append(blocks, Block{
							Kind: kind, Text: paragraph.Text,
							HeadingPath: append([]string(nil), headingPath...),
							PageFrom:    page, PageTo: page, Order: order,
						})
						order++
					}
					paragraph = nil
				}
			case "tc":
				if tableDepth > 0 {
					currentRow = append(currentRow, normalizeSpace(currentCell.String()))
				}
			case "tr":
				if tableDepth > 0 && len(currentRow) > 0 {
					tableRows = append(tableRows, currentRow)
				}
			case "tbl":
				if tableDepth > 0 {
					tableDepth--
					if tableDepth == 0 && len(tableRows) > 0 {
						table := markdownTable(tableRows)
						characters += len([]rune(table))
						if characters > limits.MaxCharacters {
							return Document{}, ErrParseLimit
						}
						blocks = append(blocks, Block{
							Kind: BlockTable, Text: table,
							HeadingPath: append([]string(nil), headingPath...),
							PageFrom:    page, PageTo: page, Order: order,
						})
						order++
					}
				}
			}
		}
	}
	if len(blocks) == 0 {
		return Document{}, ErrOCRRequired
	}
	return Document{
		Title: title, PageCount: page, Language: detectBlockLanguage(blocks),
		Blocks: blocks, Characters: characters,
	}, nil
}

func extractPDF(ctx context.Context, path, title string, limits ExtractLimits) (Document, error) {
	timeout := time.Duration(limits.TimeoutSecs) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(runCtx, "pdftotext", "-layout", "-enc", "UTF-8", path, "-")
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return Document{}, ErrParseLimit
	}
	if err != nil {
		message := strings.ToLower(stderr.String())
		if strings.Contains(message, "password") || strings.Contains(message, "encrypted") {
			return Document{}, ErrEncrypted
		}
		return Document{}, fmt.Errorf("pdftotext: %w", err)
	}
	if len([]rune(string(output))) > limits.MaxCharacters {
		return Document{}, ErrParseLimit
	}
	rawPages := strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\f")
	if len(rawPages) > 0 && strings.TrimSpace(rawPages[len(rawPages)-1]) == "" {
		rawPages = rawPages[:len(rawPages)-1]
	}
	if len(rawPages) > limits.MaxPages {
		return Document{}, ErrParseLimit
	}
	rawPages = removeRepeatedPDFMargins(rawPages)
	var blocks []Block
	order := 0
	characters := 0
	for index, rawPage := range rawPages {
		pageBlocks := textBlocks(removeRepeatedWhitespace(rawPage), index+1)
		if len(blocks) > 0 && len(pageBlocks) > 0 &&
			blocks[len(blocks)-1].Kind == BlockParagraph &&
			pageBlocks[0].Kind == BlockParagraph &&
			continuesAcrossPage(blocks[len(blocks)-1].Text, pageBlocks[0].Text) {
			blocks[len(blocks)-1].Text = normalizeSpace(blocks[len(blocks)-1].Text + " " + pageBlocks[0].Text)
			blocks[len(blocks)-1].PageTo = index + 1
			characters += len([]rune(pageBlocks[0].Text))
			pageBlocks = pageBlocks[1:]
		}
		for _, block := range pageBlocks {
			block.Order = order
			order++
			characters += len([]rune(block.Text))
			blocks = append(blocks, block)
		}
	}
	if len(blocks) == 0 || characters < 8 {
		return Document{}, ErrOCRRequired
	}
	return Document{
		Title: title, PageCount: len(rawPages), Language: detectBlockLanguage(blocks),
		Blocks: blocks, Characters: characters,
	}, nil
}

func removeRepeatedPDFMargins(pages []string) []string {
	if len(pages) < 3 {
		return pages
	}
	counts := map[string]int{}
	pageLines := make([][]string, len(pages))
	for index, page := range pages {
		lines := strings.Split(page, "\n")
		pageLines[index] = lines
		seen := map[string]bool{}
		for _, position := range []int{0, 1, len(lines) - 2, len(lines) - 1} {
			if position < 0 || position >= len(lines) {
				continue
			}
			value := normalizeSpace(lines[position])
			if len([]rune(value)) < 3 || len([]rune(value)) > 160 || seen[value] {
				continue
			}
			seen[value] = true
			counts[value]++
		}
	}
	threshold := (len(pages) + 1) / 2
	result := make([]string, len(pages))
	for index, lines := range pageLines {
		for position, line := range lines {
			value := normalizeSpace(line)
			if counts[value] >= threshold &&
				(position <= 1 || position >= len(lines)-2) {
				lines[position] = ""
			}
		}
		result[index] = strings.Join(lines, "\n")
	}
	return result
}

func continuesAcrossPage(previous, next string) bool {
	previous = strings.TrimSpace(previous)
	next = strings.TrimSpace(next)
	if previous == "" || next == "" || strings.ContainsRune("。！？.!?:：；;", []rune(previous)[len([]rune(previous))-1]) {
		return false
	}
	first := []rune(next)[0]
	return unicode.IsLower(first) || unicode.In(first, unicode.Han)
}

func textBlocks(text string, page int) []Block {
	scanner := bufio.NewScanner(strings.NewReader(strings.ReplaceAll(text, "\r\n", "\n")))
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	var blocks []Block
	var paragraph []string
	order := 0
	flush := func() {
		value := normalizeSpace(strings.Join(paragraph, " "))
		if value != "" {
			kind := BlockParagraph
			if strings.HasPrefix(value, "#") {
				kind = BlockHeading
				value = strings.TrimSpace(strings.TrimLeft(value, "#"))
			} else if isListLine(value) {
				kind = BlockList
			}
			blocks = append(blocks, Block{
				Kind: kind, Text: value, PageFrom: page, PageTo: page, Order: order,
			})
			order++
		}
		paragraph = nil
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			flush()
			continue
		}
		paragraph = append(paragraph, line)
	}
	flush()
	return blocks
}

func headingLevel(style string) int {
	value := strings.ToLower(style)
	for _, prefix := range []string{"heading", "标题"} {
		if index := strings.Index(value, prefix); index >= 0 {
			raw := strings.TrimSpace(value[index+len(prefix):])
			if level, err := strconv.Atoi(raw); err == nil && level >= 1 && level <= 9 {
				return level
			}
		}
	}
	return 0
}

func markdownTable(rows [][]string) string {
	width := 0
	for _, row := range rows {
		if len(row) > width {
			width = len(row)
		}
	}
	if width == 0 {
		return ""
	}
	normalize := func(row []string) []string {
		result := make([]string, width)
		for index := range result {
			if index < len(row) {
				result[index] = strings.ReplaceAll(strings.TrimSpace(row[index]), "|", "\\|")
			}
		}
		return result
	}
	var output strings.Builder
	header := normalize(rows[0])
	output.WriteString("| " + strings.Join(header, " | ") + " |\n")
	output.WriteString("| " + strings.TrimSuffix(strings.Repeat("--- | ", width), " ") + "\n")
	for _, row := range rows[1:] {
		output.WriteString("| " + strings.Join(normalize(row), " | ") + " |\n")
	}
	return strings.TrimSpace(output.String())
}

func normalizeSpace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func removeRepeatedWhitespace(value string) string {
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		lines[index] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

func isListLine(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") ||
		strings.HasPrefix(trimmed, "• ") || (len(trimmed) > 2 && unicode.IsDigit(rune(trimmed[0])) && trimmed[1] == '.')
}

func detectBlockLanguage(blocks []Block) string {
	var text strings.Builder
	for _, block := range blocks {
		text.WriteString(block.Text)
		if text.Len() > 4096 {
			break
		}
	}
	return detectLanguage(text.String())
}

func detectLanguage(value string) string {
	var cjk, latin int
	for _, item := range value {
		switch {
		case unicode.In(item, unicode.Han):
			cjk++
		case unicode.IsLetter(item):
			latin++
		}
	}
	if cjk > 0 && latin > 0 {
		return "zh-en"
	}
	if cjk > 0 {
		return "zh"
	}
	if latin > 0 {
		return "en"
	}
	return "und"
}
