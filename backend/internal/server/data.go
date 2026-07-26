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
    "path"
    "path/filepath"
    "regexp"
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
    writeZip(w, "diary-listener-markdown.zip", output.Bytes())
}

func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
    principal := principalFrom(r.Context())
    data, attachments, err := s.store.BackupSnapshot(r.Context(), principal)
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    files := make(map[string][]byte)
    for _, item := range attachments {
        target, err := s.safeDataPath(item.StoredPath, "attachments")
        if err != nil {
            httpx.WriteError(w, s.logger, err)
            return
        }
        content, err := os.ReadFile(target)
        if err != nil {
            httpx.WriteError(w, s.logger, err)
            return
        }
        name := path.Base(strings.ReplaceAll(item.OriginalName, "\\", "/"))
        archiveName := fmt.Sprintf("attachments/%d/%s", item.ID, name)
        digest := sha256.Sum256(content)
        files[archiveName] = content
        data.Attachments = append(data.Attachments, store.BackupAttachment{
            ID: item.ID, NoteID: item.NoteID, Name: item.OriginalName,
            MIME: item.MIMEType, SHA256: hex.EncodeToString(digest[:]), Archive: archiveName,
        })
    }
    payload, err := json.Marshal(data)
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    files["data.json"] = payload
    manifest := make(map[string]string, len(files))
    for name, content := range files {
        digest := sha256.Sum256(content)
        manifest[name] = hex.EncodeToString(digest[:])
    }
    manifestJSON, err := json.Marshal(manifest)
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    var output bytes.Buffer
    archive := zip.NewWriter(&output)
    for name, content := range files {
        writer, err := archive.Create(name)
        if err != nil {
            httpx.WriteError(w, s.logger, err)
            return
        }
        if _, err := writer.Write(content); err != nil {
            httpx.WriteError(w, s.logger, err)
            return
        }
    }
    writer, err := archive.Create("manifest.json")
    if err == nil {
        _, err = writer.Write(manifestJSON)
    }
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    if err := archive.Close(); err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    writeZip(w, "diary-listener-backup.zip", output.Bytes())
}

func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
    r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxBackupBytes+(1<<20))
    file, _, err := r.FormFile("file")
    if err != nil {
        httpx.WriteError(w, s.logger, apierror.Validation(nil))
        return
    }
    defer file.Close()
    blob, err := io.ReadAll(io.LimitReader(file, s.cfg.MaxBackupBytes+1))
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    if int64(len(blob)) > s.cfg.MaxBackupBytes {
        httpx.WriteError(w, s.logger, apierror.New("BACKUP_TOO_LARGE", "备份包过大", 413))
        return
    }
    data, files, err := validateBackup(blob, s.cfg.MaxBackupBytes)
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    if data.Version != 1 {
        httpx.WriteError(w, s.logger, apierror.New("UNSUPPORTED_BACKUP_VERSION", "不支持的备份版本", 422))
        return
    }
    principal := principalFrom(r.Context())
    restored := make([]store.RestoredAttachment, 0, len(data.Attachments))
    written := make([]string, 0, len(data.Attachments))
    cleanup := func() {
        for _, filename := range written {
            _ = os.Remove(filename)
        }
    }
    for _, source := range data.Attachments {
        content, ok := files[source.Archive]
        if !ok {
            cleanup()
            httpx.WriteError(w, s.logger, apierror.New("INVALID_BACKUP", "备份附件缺失", 422))
            return
        }
        digest := sha256.Sum256(content)
        digestText := hex.EncodeToString(digest[:])
        if digestText != source.SHA256 {
            cleanup()
            httpx.WriteError(w, s.logger, apierror.New("BACKUP_INTEGRITY_ERROR", "附件摘要不匹配", 422))
            return
        }
        extension, mimeType, validationErr := validateAttachment(source.Name, content)
        if validationErr != nil {
            cleanup()
            httpx.WriteError(w, s.logger, validationErr)
            return
        }
        now := time.Now().UTC()
        relative := filepath.Join(
            "attachments", principal.TenantID.String(), now.Format("2006"), now.Format("01"),
            uuid.NewString()+extension,
        )
        target, err := s.safeDataPath(relative, "attachments")
        if err != nil {
            cleanup()
            httpx.WriteError(w, s.logger, err)
            return
        }
        if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
            cleanup()
            httpx.WriteError(w, s.logger, err)
            return
        }
        if err := os.WriteFile(target, content, 0640); err != nil {
            cleanup()
            httpx.WriteError(w, s.logger, err)
            return
        }
        written = append(written, target)
        restored = append(restored, store.RestoredAttachment{
            SourceNoteID: source.NoteID, Name: path.Base(strings.ReplaceAll(source.Name, "\\", "/")),
            StoredPath: filepath.ToSlash(relative), MIMEType: mimeType,
            Size: int64(len(content)), SHA256: digestText,
        })
    }
    result, err := s.store.RestoreSnapshot(r.Context(), principal, data, restored)
    if err != nil {
        cleanup()
        httpx.WriteError(w, s.logger, err)
        return
    }
    httpx.JSON(w, http.StatusOK, result)
}

func validateBackup(blob []byte, maxUncompressed int64) (store.BackupData, map[string][]byte, error) {
    var data store.BackupData
    reader, err := zip.NewReader(bytes.NewReader(blob), int64(len(blob)))
    if err != nil {
        return data, nil, apierror.New("INVALID_BACKUP", "备份包格式无效", 422)
    }
    files := make(map[string][]byte)
    var manifestBytes []byte
    var total int64
    for _, entry := range reader.File {
        if !safeArchiveName(entry.Name) {
            return data, nil, apierror.New("UNSAFE_BACKUP_PATH", "备份包包含非法路径", 422)
        }
        if _, exists := files[entry.Name]; exists || (entry.Name == "manifest.json" && manifestBytes != nil) {
            return data, nil, apierror.New("INVALID_BACKUP", "备份包包含重复文件", 422)
        }
        total += int64(entry.UncompressedSize64)
        if total > maxUncompressed {
            return data, nil, apierror.New("BACKUP_TOO_LARGE", "备份包解压后过大", 413)
        }
        stream, err := entry.Open()
        if err != nil {
            return data, nil, apierror.New("INVALID_BACKUP", "备份包格式无效", 422)
        }
        content, readErr := io.ReadAll(io.LimitReader(stream, maxUncompressed+1))
        closeErr := stream.Close()
        if readErr != nil || closeErr != nil {
            return data, nil, apierror.New("INVALID_BACKUP", "备份包格式无效", 422)
        }
        if entry.Name == "manifest.json" {
            manifestBytes = content
        } else {
            files[entry.Name] = content
        }
    }
    dataBytes, dataOK := files["data.json"]
    if len(manifestBytes) == 0 || !dataOK {
        return data, nil, apierror.New("INVALID_BACKUP", "备份包缺少清单", 422)
    }
    var manifest map[string]string
    if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
        return data, nil, apierror.New("INVALID_BACKUP", "备份清单格式无效", 422)
    }
    for name, expected := range manifest {
        content, ok := files[name]
        digest := sha256.Sum256(content)
        if !ok || hex.EncodeToString(digest[:]) != expected {
            return data, nil, apierror.New("BACKUP_INTEGRITY_ERROR", "备份完整性校验失败", 422)
        }
    }
    for name := range files {
        if _, ok := manifest[name]; !ok {
            return data, nil, apierror.New("BACKUP_INTEGRITY_ERROR", "备份存在未校验文件", 422)
        }
    }
    if err := json.Unmarshal(dataBytes, &data); err != nil {
        return data, nil, apierror.New("INVALID_BACKUP", "备份数据格式无效", 422)
    }
    return data, files, nil
}

func safeArchiveName(name string) bool {
    return name != "" && !strings.Contains(name, "\\") && !strings.HasPrefix(name, "/") &&
        path.Clean(name) == name && name != ".." && !strings.HasPrefix(name, "../")
}

func writeZip(w http.ResponseWriter, filename string, content []byte) {
    w.Header().Set("Content-Type", "application/zip")
    w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
    w.WriteHeader(http.StatusOK)
    _, _ = w.Write(content)
}
