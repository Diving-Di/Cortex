package documentparser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

type Block struct {
	Type          string   `json:"block_type"`
	Text          string   `json:"text"`
	HeadingPath   []string `json:"heading_path"`
	Page          *int     `json:"page"`
	ImageIndex    *int     `json:"image_index"`
	SourceSpan    string   `json:"source_span"`
	ParserVersion string   `json:"parser_version"`
	OCRConfidence *float64 `json:"ocr_confidence"`
}

type Result struct {
	Blocks []Block `json:"blocks"`
}

type Client struct {
	BaseURL string
	Timeout time.Duration
	MaxBody int64
}

type Error struct {
	Code string
}

func (e *Error) Error() string { return e.Code }

func (c Client) Parse(ctx context.Context, filename string, data []byte) (Result, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return Result{}, fmt.Errorf("document parser is not configured")
	}
	if c.MaxBody > 0 && int64(len(data)) > c.MaxBody {
		return Result{}, fmt.Errorf("document exceeds parser limit")
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/v1/parse", bytes.NewReader(data))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	filename = filepath.Base(strings.ReplaceAll(filename, "\\", "/"))
	if filename == "." || filename == "" || strings.ContainsAny(filename, "\r\n") {
		return Result{}, &Error{Code: "KNOWLEDGE_DOCUMENT_UNSAFE"}
	}
	req.Header.Set("X-Filename", filename)
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		codes := map[int]string{400: "KNOWLEDGE_FILE_TYPE_MISMATCH", 413: "KNOWLEDGE_QUOTA_EXCEEDED", 422: "KNOWLEDGE_DOCUMENT_UNSAFE", 424: "KNOWLEDGE_OCR_FAILED"}
		if code := codes[resp.StatusCode]; code != "" {
			return Result{}, &Error{Code: code}
		}
		return Result{}, &Error{Code: "KNOWLEDGE_PARSER_UNAVAILABLE"}
	}
	var result Result
	limit := c.MaxBody
	if limit <= 0 {
		limit = 64 << 20
	}
	reader := io.LimitReader(resp.Body, limit+1)
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&result); err != nil {
		return Result{}, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return Result{}, &Error{Code: "KNOWLEDGE_PARSER_UNAVAILABLE"}
	}
	if len(result.Blocks) == 0 {
		return Result{}, &Error{Code: "KNOWLEDGE_DOCUMENT_EMPTY"}
	}
	return result, nil
}

func Markdown(result Result) string {
	var out strings.Builder
	for _, block := range result.Blocks {
		text := strings.TrimSpace(block.Text)
		if text == "" {
			continue
		}
		label := ""
		if block.Page != nil {
			label = fmt.Sprintf("第 %d 页", *block.Page)
		} else if block.ImageIndex != nil {
			label = fmt.Sprintf("图片 %d", *block.ImageIndex)
		}
		if label != "" {
			out.WriteString("\n## ")
			out.WriteString(label)
			out.WriteString("\n")
		}
		out.WriteString(text)
		out.WriteString("\n")
	}
	return strings.TrimSpace(out.String())
}
