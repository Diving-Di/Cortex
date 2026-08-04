package knowledge

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func testLimits() Limits {
	return Limits{MaxUploadBytes: 1 << 20, MaxExtractedBytes: 1 << 20, MaxFileBytes: 1 << 18, MaxFiles: 20, MaxDepth: 5, MaxCompressionRatio: 100}
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
func TestPrepareZIPRejectsTraversalAndUnsupportedFiles(t *testing.T) {
	for name, entries := range map[string]map[string][]byte{"traversal": {"../secret.md": []byte("# no")}, "unsupported": {"ok.md": []byte("# ok"), "bad.exe": []byte("MZ")}} {
		t.Run(name, func(t *testing.T) {
			_, err := Prepare(writeTestZIP(t, entries), "input.zip", filepath.Join(t.TempDir(), "source"), testLimits())
			if err == nil {
				t.Fatal("expected rejection")
			}
		})
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
