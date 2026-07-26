package server

import (
    "crypto/sha256"
    "encoding/hex"
    "errors"
    "io"
    "mime"
    "net/http"
    "os"
    "path/filepath"
    "strconv"
    "strings"
    "time"
    "unicode/utf8"

    "diary-listener/backend/internal/apierror"
    "diary-listener/backend/internal/httpx"
    "diary-listener/backend/internal/store"
    "github.com/google/uuid"
)

func (s *Server) uploadAttachment(w http.ResponseWriter, r *http.Request) {
    noteIDRaw := r.URL.Query().Get("note_id")
    noteID, err := parsePositiveInt32(noteIDRaw)
    if err != nil {
        httpx.WriteError(w, s.logger, apierror.Validation(nil))
        return
    }
    r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxAttachmentBytes+(1<<20))
    file, header, err := r.FormFile("file")
    if err != nil {
        httpx.WriteError(w, s.logger, apierror.Validation(nil))
        return
    }
    defer file.Close()
    data, err := io.ReadAll(io.LimitReader(file, s.cfg.MaxAttachmentBytes+1))
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    if int64(len(data)) > s.cfg.MaxAttachmentBytes {
        httpx.WriteError(w, s.logger, apierror.New("ATTACHMENT_TOO_LARGE", "附件超过单文件大小限制", 413))
        return
    }
    extension, mimeType, validationErr := validateAttachment(header.Filename, data)
    if validationErr != nil {
        httpx.WriteError(w, s.logger, validationErr)
        return
    }
    principal := principalFrom(r.Context())
    now := time.Now().UTC()
    relative := filepath.Join(
        "attachments", principal.TenantID.String(), now.Format("2006"), now.Format("01"),
        uuid.NewString()+extension,
    )
    target, err := s.safeDataPath(relative, "attachments")
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    if err := os.WriteFile(target, data, 0640); err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    digest := sha256.Sum256(data)
    originalName := filepath.Base(header.Filename)
    originalName = truncateRunes(originalName, 255)
    item, err := s.store.AddAttachment(r.Context(), principal, store.Attachment{
        NoteID: noteID, OriginalName: originalName,
        StoredPath: filepath.ToSlash(relative), MIMEType: mimeType,
        Size: int64(len(data)), SHA256: hex.EncodeToString(digest[:]),
    })
    if err != nil {
        _ = os.Remove(target)
        httpx.WriteError(w, s.logger, err)
        return
    }
    httpx.JSON(w, http.StatusCreated, item.Response())
}

func (s *Server) listAttachments(w http.ResponseWriter, r *http.Request) {
    noteID, err := pathID(r, "noteID")
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    items, err := s.store.ListAttachments(r.Context(), principalFrom(r.Context()), noteID)
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    result := make([]store.AttachmentResponse, 0, len(items))
    for _, item := range items {
        result = append(result, item.Response())
    }
    httpx.JSON(w, http.StatusOK, result)
}

func (s *Server) downloadAttachment(w http.ResponseWriter, r *http.Request) {
    attachmentID, err := pathID(r, "attachmentID")
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    item, err := s.store.GetAttachment(r.Context(), principalFrom(r.Context()), attachmentID)
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    path, err := s.safeDataPath(item.StoredPath, "attachments")
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    file, err := os.Open(path)
    if errors.Is(err, os.ErrNotExist) {
        httpx.WriteError(w, s.logger, apierror.New("ATTACHMENT_FILE_MISSING", "附件文件缺失", 410))
        return
    }
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    defer file.Close()
    stat, err := file.Stat()
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    w.Header().Set("Content-Type", item.MIMEType)
    w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": item.OriginalName}))
    http.ServeContent(w, r, item.OriginalName, stat.ModTime(), file)
}

func (s *Server) deleteAttachment(w http.ResponseWriter, r *http.Request) {
    attachmentID, err := pathID(r, "attachmentID")
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    principal := principalFrom(r.Context())
    item, err := s.store.GetAttachment(r.Context(), principal, attachmentID)
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    path, err := s.safeDataPath(item.StoredPath, "attachments")
    if err != nil {
        httpx.WriteError(w, s.logger, err)
        return
    }
    tombstone := path + ".deleting"
    moved := false
    if err := os.Rename(path, tombstone); err == nil {
        moved = true
    } else if !errors.Is(err, os.ErrNotExist) {
        httpx.WriteError(w, s.logger, err)
        return
    }
    if err := s.store.DeleteAttachment(r.Context(), principal, attachmentID); err != nil {
        if moved {
            _ = os.Rename(tombstone, path)
        }
        httpx.WriteError(w, s.logger, err)
        return
    }
    if moved {
        _ = os.Remove(tombstone)
    }
    w.WriteHeader(http.StatusNoContent)
}

func validateAttachment(filename string, data []byte) (string, string, error) {
    if len(data) == 0 {
        return "", "", apierror.New("EMPTY_FILE", "附件不能为空", 422)
    }
    extension := strings.ToLower(filepath.Ext(filename))
    signatures := map[string]struct {
        mime string
        data []byte
    }{
        ".png":  {"image/png", []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}},
        ".jpg":  {"image/jpeg", []byte{0xff, 0xd8, 0xff}},
        ".jpeg": {"image/jpeg", []byte{0xff, 0xd8, 0xff}},
        ".pdf":  {"application/pdf", []byte("%PDF-")},
    }
    if signature, ok := signatures[extension]; ok {
        if !strings.HasPrefix(string(data), string(signature.data)) {
            return "", "", apierror.New("INVALID_FILE_SIGNATURE", "文件内容与扩展名不匹配", 422)
        }
        return extension, signature.mime, nil
    }
    textTypes := map[string]string{".txt": "text/plain", ".md": "text/markdown"}
    if mimeType, ok := textTypes[extension]; ok {
        if !utf8.Valid(data) {
            return "", "", apierror.New("INVALID_FILE_SIGNATURE", "文本附件必须为 UTF-8", 422)
        }
        return extension, mimeType, nil
    }
    return "", "", apierror.New("UNSUPPORTED_FILE_TYPE", "不支持的附件类型", 422)
}

func (s *Server) safeDataPath(relative, expectedRoot string) (string, error) {
    cleaned := filepath.Clean(filepath.FromSlash(relative))
    if filepath.IsAbs(cleaned) || cleaned == "." || cleaned == ".." ||
        !strings.HasPrefix(cleaned, expectedRoot+string(filepath.Separator)) {
        return "", apierror.New("INVALID_ATTACHMENT_PATH", "附件路径无效", 500)
    }
    root := filepath.Join(s.cfg.DataDir, expectedRoot)
    target := filepath.Join(s.cfg.DataDir, cleaned)
    relativeToRoot, err := filepath.Rel(root, target)
    if err != nil || relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, ".."+string(filepath.Separator)) {
        return "", apierror.New("INVALID_ATTACHMENT_PATH", "附件路径无效", 500)
    }
    return target, nil
}

func truncateRunes(value string, limit int) string {
    runes := []rune(value)
    if len(runes) > limit {
        return string(runes[:limit])
    }
    return value
}

func parsePositiveInt32(raw string) (int32, error) {
    value, err := strconv.ParseInt(raw, 10, 32)
    if err != nil || value <= 0 {
        return 0, apierror.Validation(nil)
    }
    return int32(value), nil
}
