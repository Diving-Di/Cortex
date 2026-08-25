package knowledge

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func testLimits() Limits {
	return Limits{MaxUploadBytes: 1 << 20, MaxExtractedBytes: 1 << 20, MaxFileBytes: 1 << 18, MaxFiles: 20, MaxDepth: 5, MaxCompressionRatio: 100}
}
func testImage(t *testing.T, format string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var out bytes.Buffer
	var err error
	if format == "png" {
		err = png.Encode(&out, img)
	} else {
		err = jpeg.Encode(&out, img, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func writeTestZIP(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "test.zip")
	file, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for path, data := range entries {
		entry, err := writer.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return name
}
func TestPrepareMarkdownNormalizesEncodingAndNewlines(t *testing.T) {
	input := filepath.Join(t.TempDir(), "input.md")
	if err := os.WriteFile(input, append([]byte{0xef, 0xbb, 0xbf}, []byte("# 标题\r\n正文\r")...), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "source")
	got, err := Prepare(input, "知识.md", root, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if got.Documents[0].Encoding != "utf-8-bom" {
		t.Fatalf("encoding=%q", got.Documents[0].Encoding)
	}
	data, err := os.ReadFile(filepath.Join(root, "知识.md"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("\r")) || bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		t.Fatalf("not normalized: %q", data)
	}
}

func TestPrepareBinaryDocuments(t *testing.T) {
	docx := writeTestZIP(t, map[string][]byte{"[Content_Types].xml": []byte("<Types/>"), "word/document.xml": []byte("<document/>")})
	docxData, err := os.ReadFile(docx)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		data []byte
	}{
		{"report.pdf", []byte("%PDF-1.7\n")},
		{"legacy.doc", append([]byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}, make([]byte, 16)...)},
		{"report.docx", docxData},
		{"scan.png", testImage(t, "png")},
		{"photo.jpeg", testImage(t, "jpg")},
		{"scan.webp", append([]byte("RIFF\x04\x00\x00\x00WEBP"), []byte("VP8 ")...)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := filepath.Join(t.TempDir(), "upload")
			if err := os.WriteFile(input, tc.data, 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := Prepare(input, tc.name, filepath.Join(t.TempDir(), "source"), testLimits())
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Documents) != 1 || got.Documents[0].Encoding != "binary" {
				t.Fatalf("documents=%+v", got.Documents)
			}
		})
	}
}

func TestPrepareBinaryRejectsExtensionMagicMismatch(t *testing.T) {
	input := filepath.Join(t.TempDir(), "upload")
	if err := os.WriteFile(input, []byte("not a PDF"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Prepare(input, "report.pdf", filepath.Join(t.TempDir(), "source"), testLimits())
	if !IsCode(err, "KNOWLEDGE_FILE_TYPE_MISMATCH") {
		t.Fatalf("error=%v", err)
	}
}
func TestPrepareZIPRejectsTraversal(t *testing.T) {
	_, err := Prepare(writeTestZIP(t, map[string][]byte{"../secret.md": []byte("# no")}), "input.zip", filepath.Join(t.TempDir(), "source"), testLimits())
	if err == nil {
		t.Fatal("expected rejection")
	}
}
func TestPrepareZIPSkipsUnsupportedFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "source")
	got, err := Prepare(writeTestZIP(t, map[string][]byte{
		"ok.md":        []byte("# ok"),
		"bad.exe":      []byte("MZ"),
		"document.pdf": []byte("%PDF"),
		"page.html":    []byte("<html></html>"),
		"image.jpeg":   []byte("ignored"),
		"image.gif":    []byte("ignored"),
		"image.webp":   []byte("ignored"),
	}), "input.zip", root, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Documents) != 1 || len(got.Assets) != 0 {
		t.Fatalf("documents=%d assets=%d", len(got.Documents), len(got.Assets))
	}
	for _, name := range []string{"bad.exe", "document.pdf", "page.html", "image.jpeg", "image.gif", "image.webp"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("skipped entry %q was written or stat failed: %v", name, err)
		}
	}
}
func TestPrepareZIPAcceptsPNGAndJPG(t *testing.T) {
	got, err := Prepare(writeTestZIP(t, map[string][]byte{
		"ok.md":     []byte("# ok"),
		"image.png": testImage(t, "png"),
		"image.jpg": testImage(t, "jpg"),
	}), "input.zip", filepath.Join(t.TempDir(), "source"), testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Documents) != 1 || len(got.Assets) != 2 {
		t.Fatalf("documents=%d assets=%d", len(got.Documents), len(got.Assets))
	}
}
func TestPrepareZIPRequiresMarkdown(t *testing.T) {
	png := []byte("not an image")
	_, err := Prepare(writeTestZIP(t, map[string][]byte{"image.png": png}), "input.zip", filepath.Join(t.TempDir(), "source"), testLimits())
	if err == nil {
		t.Fatal("expected rejection")
	}
}
func TestChunkIncludesSourceAndHeading(t *testing.T) {
	chunks := Chunk("设计", "note", "# 总览\n正文\n## 安全\n租户隔离")
	if len(chunks) != 2 {
		t.Fatalf("parents=%d", len(chunks))
	}
	if !bytes.Contains([]byte(chunks[1].Children[0].EmbeddingText), []byte("来源：note")) {
		t.Fatal("source missing")
	}
}
