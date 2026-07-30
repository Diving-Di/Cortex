package server

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/httpx"
	"diary-listener/backend/internal/store"
	"github.com/google/uuid"
)

var unsafeFilename = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

func (s *Server) exportMarkdown(w http.ResponseWriter, r *http.Request) {
	notes, err := s.store.ExportNotes(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	used := make(map[string]bool)
	for _, note := range notes {
		base := strings.Trim(unsafeFilename.ReplaceAllString(note.Title, "_"), " .")
		if base == "" {
			base = fmt.Sprintf("note-%d", note.ID)
		}
		date := "undated"
		if note.NoteDate != nil {
			date = *note.NoteDate
		}
		original := date + "-" + base + ".md"
		candidate := original
		for index := 2; used[strings.ToLower(candidate)]; index++ {
			candidate = strings.TrimSuffix(original, ".md") + fmt.Sprintf("-%d.md", index)
		}
		used[strings.ToLower(candidate)] = true
		writer, err := archive.Create(candidate)
		if err != nil {
			httpx.WriteError(w, s.logger, err)
			return
		}
		if _, err := writer.Write([]byte("# " + note.Title + "\n\n" + note.Content)); err != nil {
			httpx.WriteError(w, s.logger, err)
			return
		}
	}
	if err := archive.Close(); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	writeZip(w, "cortex-markdown.zip", output.Bytes())
}

func writeZip(w http.ResponseWriter, filename string, content []byte) {
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

type backupFileSpec struct {
	table       string
	pathField   string
	archiveRoot string
}

var backupFileSpecs = []backupFileSpec{
	{table: "attachments", pathField: "stored_path", archiveRoot: "attachments"},
	{table: "research_assets", pathField: "storage_path", archiveRoot: "research"},
}

func (s *Server) exportFullBackup(w http.ResponseWriter, r *http.Request) {
	backup, err := s.store.ExportFullBackup(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	output, err := os.CreateTemp("", "cortex-backup-*.zip")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	defer func() {
		_ = output.Close()
		_ = os.Remove(output.Name())
	}()
	archive := zip.NewWriter(output)
	for _, spec := range backupFileSpecs {
		for _, row := range backup.Tables[spec.table] {
			storedPath, _ := row[spec.pathField].(string)
			delete(row, spec.pathField)
			if storedPath == "" {
				continue
			}
			source, pathErr := s.safeDataPath(storedPath, spec.archiveRoot)
			if pathErr != nil {
				httpx.WriteError(w, s.logger, pathErr)
				return
			}
			content, readErr := os.Open(source)
			if readErr != nil {
				httpx.WriteError(w, s.logger, apierror.New("BACKUP_FILE_UNAVAILABLE", "备份所需文件不可用", 409))
				return
			}
			entry := fmt.Sprintf("files/%s/%s", spec.table, backupIDKey(row["id"]))
			writer, createErr := archive.CreateHeader(&zip.FileHeader{Name: entry, Method: zip.Deflate})
			if createErr != nil {
				httpx.WriteError(w, s.logger, createErr)
				return
			}
			_, writeErr := io.Copy(writer, content)
			_ = content.Close()
			if writeErr != nil {
				httpx.WriteError(w, s.logger, writeErr)
				return
			}
			row["_file"] = entry
		}
	}
	manifest, err := json.Marshal(backup)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	writer, err := archive.Create("manifest.json")
	if err == nil {
		_, err = writer.Write(manifest)
	}
	if err == nil {
		err = archive.Close()
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if _, err := output.Seek(0, io.SeekStart); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "cortex-full-backup.zip"}))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, output)
}

func (s *Server) restoreFullBackup(w http.ResponseWriter, r *http.Request) {
	const maxSize int64 = 4 << 30
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)
	payload, err := os.CreateTemp("", "cortex-restore-*.zip")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	payloadName := payload.Name()
	defer func() {
		_ = payload.Close()
		_ = os.Remove(payloadName)
	}()
	if _, err = io.Copy(payload, r.Body); err != nil {
		httpx.WriteError(w, s.logger, apierror.New("BACKUP_TOO_LARGE", "备份包超过恢复限制", 413))
		return
	}
	if err = payload.Close(); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	reader, err := zip.OpenReader(payloadName)
	if err != nil {
		httpx.WriteError(w, s.logger, apierror.New("BACKUP_INVALID", "备份包不是有效 ZIP", 422))
		return
	}
	defer reader.Close()
	entries := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		cleaned := filepath.ToSlash(filepath.Clean(file.Name))
		if cleaned != file.Name || strings.HasPrefix(cleaned, "../") || filepath.IsAbs(file.Name) {
			httpx.WriteError(w, s.logger, apierror.New("BACKUP_INVALID", "备份包包含不安全路径", 422))
			return
		}
		entries[file.Name] = file
	}
	manifestFile := entries["manifest.json"]
	if manifestFile == nil {
		httpx.WriteError(w, s.logger, apierror.New("BACKUP_INVALID", "备份包缺少清单", 422))
		return
	}
	manifestReader, err := manifestFile.Open()
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	decoder := json.NewDecoder(io.LimitReader(manifestReader, 32<<20))
	decoder.UseNumber()
	var backup store.FullBackup
	err = decoder.Decode(&backup)
	_ = manifestReader.Close()
	if err != nil {
		httpx.WriteError(w, s.logger, apierror.New("BACKUP_INVALID", "备份清单无效", 422))
		return
	}
	principal := principalFrom(r.Context())
	var created []string
	defer func() {
		if err != nil {
			for _, path := range created {
				_ = os.Remove(path)
			}
		}
	}()
	for _, spec := range backupFileSpecs {
		for _, row := range backup.Tables[spec.table] {
			entryName, _ := row["_file"].(string)
			delete(row, "_file")
			entry := entries[entryName]
			if entry == nil {
				err = apierror.New("BACKUP_FILE_UNAVAILABLE", "备份清单引用的文件不存在", 422)
				httpx.WriteError(w, s.logger, err)
				return
			}
			source, openErr := entry.Open()
			if openErr != nil {
				err = openErr
				httpx.WriteError(w, s.logger, err)
				return
			}
			extension := filepath.Ext(fmt.Sprint(row["original_name"]))
			if spec.table == "research_assets" {
				extension = extensionForMIME(fmt.Sprint(row["mime_type"]))
			}
			now := time.Now().UTC()
			relative := filepath.Join(spec.archiveRoot, principal.TenantID.String(), now.Format("2006"),
				now.Format("01"), uuid.NewString()+extension)
			if spec.table == "research_assets" {
				relative = filepath.Join(spec.archiveRoot, principal.TenantID.String(), "restored", uuid.NewString()+extension)
			}
			target, pathErr := s.safeDataPath(relative, spec.archiveRoot)
			if pathErr != nil {
				_ = source.Close()
				err = pathErr
				httpx.WriteError(w, s.logger, err)
				return
			}
			if mkdirErr := os.MkdirAll(filepath.Dir(target), 0750); mkdirErr != nil {
				_ = source.Close()
				err = mkdirErr
				httpx.WriteError(w, s.logger, err)
				return
			}
			hasher := sha256.New()
			temp, createErr := os.CreateTemp(filepath.Dir(target), ".restore-*")
			if createErr != nil {
				_ = source.Close()
				err = createErr
				httpx.WriteError(w, s.logger, err)
				return
			}
			sizeField := "size"
			if spec.table == "research_assets" {
				sizeField = "byte_size"
			}
			expectedSize, sizeErr := strconv.ParseInt(fmt.Sprint(row[sizeField]), 10, 64)
			if sizeErr != nil || expectedSize <= 0 || uint64(expectedSize) != entry.UncompressedSize64 {
				_ = source.Close()
				_ = temp.Close()
				_ = os.Remove(temp.Name())
				err = apierror.New("BACKUP_FILE_CORRUPT", "备份文件大小无效", 422)
				httpx.WriteError(w, s.logger, err)
				return
			}
			written, copyErr := io.Copy(io.MultiWriter(temp, hasher), io.LimitReader(source, expectedSize+1))
			_ = source.Close()
			closeErr := temp.Close()
			if copyErr != nil || closeErr != nil || written != expectedSize ||
				!strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), strings.TrimSpace(fmt.Sprint(row["sha256"]))) {
				_ = os.Remove(temp.Name())
				err = apierror.New("BACKUP_FILE_CORRUPT", "备份文件校验失败", 422)
				httpx.WriteError(w, s.logger, err)
				return
			}
			if renameErr := os.Rename(temp.Name(), target); renameErr != nil {
				_ = os.Remove(temp.Name())
				err = renameErr
				httpx.WriteError(w, s.logger, err)
				return
			}
			created = append(created, target)
			row[spec.pathField] = filepath.ToSlash(relative)
		}
	}
	err = s.store.RestoreFullBackup(r.Context(), principal, backup)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]string{"status": "restored"})
}

func backupIDKey(value interface{}) string {
	if number, ok := value.(json.Number); ok {
		return number.String()
	}
	return fmt.Sprint(value)
}

func extensionForMIME(value string) string {
	switch value {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ".bin"
	}
}
