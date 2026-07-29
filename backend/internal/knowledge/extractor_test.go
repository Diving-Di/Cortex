package knowledge

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractDOCXPreservesHeadingListAndTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.docx")
	documentXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>成长目标</w:t></w:r></w:p>
<w:p><w:pPr><w:numPr/></w:pPr><w:r><w:t>每天复盘</w:t></w:r></w:p>
<w:tbl><w:tr><w:tc><w:p><w:r><w:t>日期</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>证据</w:t></w:r></w:p></w:tc></w:tr>
<w:tr><w:tc><w:p><w:r><w:t>周一</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>完成</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
</w:body></w:document>`
	writeTestDOCX(t, path, documentXML)

	document, err := Extract(context.Background(), path, ".docx", "sample", ExtractLimits{
		MaxPages: 10, MaxCharacters: 10_000, TimeoutSecs: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Blocks) != 3 || document.Blocks[0].Kind != BlockHeading ||
		document.Blocks[1].Kind != BlockList || document.Blocks[2].Kind != BlockTable {
		t.Fatalf("unexpected blocks: %#v", document.Blocks)
	}
	if len(document.Blocks[1].HeadingPath) != 1 ||
		document.Blocks[1].HeadingPath[0] != "成长目标" {
		t.Fatalf("heading path lost: %#v", document.Blocks[1].HeadingPath)
	}
}

func TestRemoveRepeatedPDFMarginsAndJoinCrossPage(t *testing.T) {
	pages := []string{
		"Cortex 报告\n第一段没有结束\n第 1 页",
		"Cortex 报告\n继续上一页的中文内容。\n第 2 页",
		"Cortex 报告\n最后一段。\n第 3 页",
	}
	cleaned := removeRepeatedPDFMargins(pages)
	for _, page := range cleaned {
		if strings.Contains(page, "Cortex 报告") {
			t.Fatalf("repeated header remained: %q", page)
		}
	}
	if !continuesAcrossPage("第一段没有结束", "继续上一页的中文内容。") {
		t.Fatal("expected Chinese cross-page continuation")
	}
}

func TestExtractDOCXRejectsMalformedAndCharacterLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "malformed.docx")
	writeTestDOCX(t, path, `<w:document><w:body><w:p>`)
	if _, err := Extract(context.Background(), path, ".docx", "bad", ExtractLimits{
		MaxPages: 10, MaxCharacters: 100, TimeoutSecs: 1,
	}); err == nil {
		t.Fatal("malformed DOCX accepted")
	}

	path = filepath.Join(t.TempDir(), "large.docx")
	writeTestDOCX(t, path, `<?xml version="1.0"?><w:document xmlns:w="x"><w:body>
<w:p><w:r><w:t>超过字符限制的内容</w:t></w:r></w:p></w:body></w:document>`)
	if _, err := Extract(context.Background(), path, ".docx", "large", ExtractLimits{
		MaxPages: 10, MaxCharacters: 2, TimeoutSecs: 1,
	}); !errors.Is(err, ErrParseLimit) {
		t.Fatalf("limit error = %v", err)
	}
}

func TestExtractTXTRejectsCharacterLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.txt")
	if err := os.WriteFile(path, []byte("超过限制"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Extract(context.Background(), path, ".txt", "large", ExtractLimits{
		MaxPages: 1, MaxCharacters: 2, TimeoutSecs: 1,
	}); !errors.Is(err, ErrParseLimit) {
		t.Fatalf("limit error = %v", err)
	}
}

func TestExtractBilingualFixedFixtures(t *testing.T) {
	fixtureDir := filepath.Join("..", "..", "testdata", "knowledge")
	tests := []struct {
		extension    string
		minPageCount int
		want         []string
	}{
		{extension: ".txt", minPageCount: 1, want: []string{"苍穹计划", "Project Sky", "当前租户"}},
		{extension: ".pdf", minPageCount: 2, want: []string{"苍穹计划", "Project Sky", "2042"}},
		{extension: ".docx", minPageCount: 1, want: []string{"苍穹计划", "Project Sky"}},
	}
	for _, test := range tests {
		t.Run(test.extension, func(t *testing.T) {
			path := filepath.Join(fixtureDir, "bilingual-sample"+test.extension)
			document, err := Extract(context.Background(), path, test.extension, "bilingual fixture", ExtractLimits{
				MaxPages: 20, MaxCharacters: 20_000, TimeoutSecs: 10,
			})
			if err != nil {
				t.Fatal(err)
			}
			if document.PageCount < test.minPageCount || document.Characters <= 0 || len(document.Blocks) == 0 {
				t.Fatalf("incomplete extraction: pages=%d chars=%d blocks=%d", document.PageCount, document.Characters, len(document.Blocks))
			}
			var extracted strings.Builder
			for _, block := range document.Blocks {
				extracted.WriteString(block.Text)
				extracted.WriteByte('\n')
			}
			for _, value := range test.want {
				if !strings.Contains(extracted.String(), value) {
					t.Fatalf("missing %q in extracted fixture", value)
				}
			}
			if document.Characters > 20_000 {
				t.Fatalf("character limit not enforced: %d", document.Characters)
			}
		})
	}
}

func writeTestDOCX(t *testing.T, path, documentXML string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	for name, value := range map[string]string{
		"[Content_Types].xml": `<?xml version="1.0"?><Types/>`,
		"word/document.xml":   documentXML,
	} {
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
