package research

import "testing"

func TestPrepareContentCleansAndMeasuresOCR(t *testing.T) {
	content, diagnostics := PrepareContent(
		" 第一行\r\n\r\n\r\n第二行\x00 ",
		[]string{" 图片一\n\n文字 ", "", "图片二"},
		"browser_detail", 2,
	)
	if content != "第一行\n\n第二行\n\n## 图片提取文字\n\n图片一\n\n文字\n\n图片二" {
		t.Fatalf("content=%q", content)
	}
	if diagnostics.ParseStrategy != "browser_detail" ||
		diagnostics.OCRContributionChars == 0 || diagnostics.ContentCompleteness <= 20 {
		t.Fatalf("diagnostics=%+v", diagnostics)
	}
}

func TestPrepareContentMetadataWithoutOCRHasLowCompleteness(t *testing.T) {
	_, diagnostics := PrepareContent("短文本", nil, "metadata", 1)
	if diagnostics.ContentCompleteness >= 50 {
		t.Fatalf("completeness=%d", diagnostics.ContentCompleteness)
	}
}
