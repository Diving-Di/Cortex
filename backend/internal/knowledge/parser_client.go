package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type ParserClient struct {
	BaseURL     string
	MaxResponse int64
	HTTPClient  *http.Client
}

type parserPage struct {
	Page     int    `json:"page"`
	Markdown string `json:"markdown"`
}

type parserResponse struct {
	Pages          []parserPage `json:"pages"`
	PageCount      int          `json:"page_count"`
	CharacterCount int          `json:"character_count"`
	Code           string       `json:"code"`
}

func (c ParserClient) Parse(ctx context.Context, path, extension, title string, limits ExtractLimits) (Document, error) {
	file, err := os.Open(path)
	if err != nil {
		return Document{}, err
	}
	defer file.Close()
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/v1/parse", file,
	)
	if err != nil {
		return Document{}, err
	}
	stat, err := file.Stat()
	if err != nil {
		return Document{}, err
	}
	request.ContentLength = stat.Size()
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-Document-Format", strings.TrimPrefix(strings.ToLower(extension), "."))
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return Document{}, fmt.Errorf("document parser unavailable: %w", err)
	}
	defer response.Body.Close()
	maxResponse := c.MaxResponse
	if maxResponse <= 0 {
		maxResponse = int64(limits.MaxCharacters)*2 + 1<<20
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil {
		return Document{}, err
	}
	if int64(len(body)) > maxResponse {
		return Document{}, ErrParseLimit
	}
	var payload parserResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return Document{}, errors.New("invalid document parser response")
	}
	if response.StatusCode != http.StatusOK {
		switch payload.Code {
		case "DOCUMENT_ENCRYPTED":
			return Document{}, ErrEncrypted
		case "DOCUMENT_OCR_REQUIRED":
			return Document{}, ErrOCRRequired
		case "DOCUMENT_PARSE_LIMIT":
			return Document{}, ErrParseLimit
		default:
			return Document{}, errors.New("document parser failed")
		}
	}
	if payload.PageCount > limits.MaxPages || payload.CharacterCount > limits.MaxCharacters {
		return Document{}, ErrParseLimit
	}
	var blocks []Block
	characters := 0
	order := 0
	for _, page := range payload.Pages {
		if page.Page <= 0 || page.Page > limits.MaxPages {
			return Document{}, errors.New("invalid document parser page")
		}
		for _, block := range textBlocks(page.Markdown, page.Page) {
			block.Order = order
			order++
			characters += len([]rune(block.Text))
			blocks = append(blocks, block)
		}
	}
	if len(blocks) == 0 {
		return Document{}, ErrOCRRequired
	}
	return Document{
		Title: title, PageCount: payload.PageCount, Language: detectBlockLanguage(blocks),
		Blocks: blocks, Characters: characters,
	}, nil
}
