package knowledge

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

type Limits struct {
	MaxUploadBytes, MaxExtractedBytes, MaxFileBytes int64
	MaxFiles, MaxDepth, MaxCompressionRatio         int
}
type Document struct {
	RelativePath, Title, Encoding, Hash string
	Size                                int64
}
type Asset struct {
	RelativePath, MIME, Hash string
	Size                     int64
}
type Prepared struct {
	Documents     []Document
	Assets        []Asset
	ExpandedBytes int64
}
type Error struct{ Code string }

func (e *Error) Error() string { return e.Code }

var imageExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true}

func Prepare(uploadPath, originalName, targetRoot string, limits Limits) (Prepared, error) {
	ext := strings.ToLower(filepath.Ext(originalName))
	if ext != ".md" && ext != ".zip" {
		return Prepared{}, &Error{Code: "KNOWLEDGE_FILE_TYPE_UNSUPPORTED"}
	}
	info, err := os.Stat(uploadPath)
	if err != nil {
		return Prepared{}, err
	}
	if info.Size() > limits.MaxUploadBytes {
		return Prepared{}, &Error{Code: "KNOWLEDGE_QUOTA_EXCEEDED"}
	}
	if err := os.MkdirAll(targetRoot, 0o750); err != nil {
		return Prepared{}, err
	}
	if ext == ".md" {
		data, err := os.ReadFile(uploadPath)
		if err != nil {
			return Prepared{}, err
		}
		return writeMarkdown(data, safeBase(originalName), targetRoot, limits)
	}
	return prepareZIP(uploadPath, targetRoot, limits)
}

func prepareZIP(filename, root string, limits Limits) (Prepared, error) {
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return Prepared{}, &Error{Code: "KNOWLEDGE_ARCHIVE_INVALID"}
	}
	defer zr.Close()
	if len(zr.File) > limits.MaxFiles {
		return Prepared{}, &Error{Code: "KNOWLEDGE_ARCHIVE_UNSAFE"}
	}
	var out Prepared
	for _, f := range zr.File {
		name, err := archiveName(f)
		if err != nil {
			return Prepared{}, err
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Mode()&os.ModeType != 0 {
			return Prepared{}, &Error{Code: "KNOWLEDGE_ARCHIVE_UNSAFE"}
		}
		clean, err := safeArchivePath(name, limits.MaxDepth)
		if err != nil {
			return Prepared{}, err
		}
		ext := strings.ToLower(path.Ext(clean))
		if ext != ".md" && !imageExt[ext] {
			return Prepared{}, &Error{Code: "KNOWLEDGE_FILE_TYPE_UNSUPPORTED"}
		}
		if int64(f.UncompressedSize64) > limits.MaxFileBytes || f.CompressedSize64 == 0 && f.UncompressedSize64 > 0 ||
			(f.CompressedSize64 > 0 && f.UncompressedSize64/f.CompressedSize64 > uint64(limits.MaxCompressionRatio)) {
			return Prepared{}, &Error{Code: "KNOWLEDGE_ARCHIVE_UNSAFE"}
		}
		if out.ExpandedBytes+int64(f.UncompressedSize64) > limits.MaxExtractedBytes {
			return Prepared{}, &Error{Code: "KNOWLEDGE_ARCHIVE_UNSAFE"}
		}
		rc, err := f.Open()
		if err != nil {
			return Prepared{}, &Error{Code: "KNOWLEDGE_ARCHIVE_INVALID"}
		}
		data, readErr := io.ReadAll(io.LimitReader(rc, limits.MaxFileBytes+1))
		closeErr := rc.Close()
		if readErr != nil || closeErr != nil || int64(len(data)) > limits.MaxFileBytes {
			return Prepared{}, &Error{Code: "KNOWLEDGE_ARCHIVE_INVALID"}
		}
		if ext == ".md" {
			p, err := writeMarkdown(data, clean, root, limits)
			if err != nil {
				return Prepared{}, fmt.Errorf("entry %q: %w", clean, err)
			}
			out.Documents = append(out.Documents, p.Documents...)
			out.ExpandedBytes += p.ExpandedBytes
		} else {
			asset, err := writeImage(data, clean, root, ext)
			if err != nil {
				return Prepared{}, fmt.Errorf("entry %q: %w", clean, err)
			}
			out.Assets = append(out.Assets, asset)
			out.ExpandedBytes += asset.Size
		}
	}
	if len(out.Documents) == 0 {
		return Prepared{}, &Error{Code: "KNOWLEDGE_ARCHIVE_INVALID"}
	}
	return out, nil
}

func archiveName(f *zip.File) (string, error) {
	raw := []byte(f.Name)
	if !f.NonUTF8 && utf8.Valid(raw) {
		return f.Name, nil
	}
	decoded, err := io.ReadAll(transform.NewReader(bytes.NewReader(raw), simplifiedchinese.GB18030.NewDecoder()))
	if err != nil || !utf8.Valid(decoded) {
		return "", &Error{Code: "KNOWLEDGE_ARCHIVE_INVALID"}
	}
	return string(decoded), nil
}

func safeArchivePath(name string, maxDepth int) (string, error) {
	if strings.ContainsRune(name, 0) || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") || strings.HasPrefix(name, "//") ||
		(len(name) >= 2 && ((name[0] >= 'A' && name[0] <= 'Z') || (name[0] >= 'a' && name[0] <= 'z')) && name[1] == ':') {
		return "", &Error{Code: "KNOWLEDGE_ARCHIVE_UNSAFE"}
	}
	name = strings.ReplaceAll(name, "\\", "/")
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || len(strings.Split(clean, "/")) > maxDepth {
		return "", &Error{Code: "KNOWLEDGE_ARCHIVE_UNSAFE"}
	}
	return clean, nil
}

func decodeMarkdown(data []byte) ([]byte, string, error) {
	if bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		return normalize(data[3:]), "utf-8-bom", nil
	}
	if utf8.Valid(data) {
		return normalize(data), "utf-8", nil
	}
	decoded, err := io.ReadAll(transform.NewReader(bytes.NewReader(data), simplifiedchinese.GB18030.NewDecoder()))
	if err != nil || !utf8.Valid(decoded) {
		return nil, "", &Error{Code: "KNOWLEDGE_ENCODING_UNSUPPORTED"}
	}
	return normalize(decoded), "gb18030", nil
}
func normalize(data []byte) []byte {
	return bytes.ReplaceAll(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")), []byte("\r"), []byte("\n"))
}
func writeMarkdown(data []byte, rel, root string, limits Limits) (Prepared, error) {
	decoded, enc, err := decodeMarkdown(data)
	if err != nil {
		return Prepared{}, err
	}
	if len(bytes.TrimSpace(decoded)) == 0 {
		return Prepared{}, &Error{Code: "KNOWLEDGE_MARKDOWN_INVALID"}
	}
	if int64(len(decoded)) > limits.MaxFileBytes {
		return Prepared{}, &Error{Code: "KNOWLEDGE_QUOTA_EXCEEDED"}
	}
	if err := writeFile(root, rel, decoded); err != nil {
		return Prepared{}, err
	}
	sum := sha256.Sum256(decoded)
	title := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	for _, line := range strings.Split(string(decoded), "\n") {
		if strings.HasPrefix(line, "# ") {
			title = strings.TrimSpace(line[2:])
			break
		}
	}
	return Prepared{Documents: []Document{{RelativePath: rel, Title: title, Encoding: enc, Hash: hex.EncodeToString(sum[:]), Size: int64(len(decoded))}}, ExpandedBytes: int64(len(decoded))}, nil
}
func writeImage(data []byte, rel, root, ext string) (Asset, error) {
	actual := http.DetectContentType(data)
	expected := mime.TypeByExtension(ext)
	if i := strings.IndexByte(expected, ';'); i >= 0 {
		expected = expected[:i]
	}
	if ext == ".jpg" || ext == ".jpeg" {
		expected = "image/jpeg"
	}
	if expected == "" || actual != expected {
		return Asset{}, &Error{Code: "KNOWLEDGE_ARCHIVE_INVALID"}
	}
	if ext != ".webp" {
		if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
			return Asset{}, &Error{Code: "KNOWLEDGE_ARCHIVE_INVALID"}
		}
	}
	if err := writeFile(root, rel, data); err != nil {
		return Asset{}, err
	}
	sum := sha256.Sum256(data)
	return Asset{RelativePath: rel, MIME: actual, Hash: hex.EncodeToString(sum[:]), Size: int64(len(data))}, nil
}
func writeFile(root, rel string, data []byte) error {
	target := filepath.Join(root, filepath.FromSlash(rel))
	absRoot, _ := filepath.Abs(root)
	absTarget, _ := filepath.Abs(target)
	prefix := absRoot + string(os.PathSeparator)
	if absTarget == absRoot || !strings.HasPrefix(absTarget, prefix) {
		return &Error{Code: "KNOWLEDGE_ARCHIVE_UNSAFE"}
	}
	if err := os.MkdirAll(filepath.Dir(absTarget), 0o750); err != nil {
		return err
	}
	return os.WriteFile(absTarget, data, 0o640)
}
func safeBase(name string) string {
	value := filepath.Base(name)
	if strings.ToLower(filepath.Ext(value)) != ".md" {
		return "document.md"
	}
	return value
}
func IsCode(err error, code string) bool {
	var target *Error
	return errors.As(err, &target) && target.Code == code
}
func ErrorCode(err error) string {
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return fmt.Sprintf("%T", err)
}
