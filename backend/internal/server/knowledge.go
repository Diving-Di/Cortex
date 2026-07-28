package server

import (
	"archive/zip"
	"bufio"
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

type knowledgeCollectionRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

func (s *Server) createKnowledgeCollection(w http.ResponseWriter, r *http.Request) {
	var request knowledgeCollectionRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if len([]rune(request.Name)) == 0 || len([]rune(request.Name)) > 120 {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	if request.Description != nil {
		value := strings.TrimSpace(*request.Description)
		if len([]rune(value)) > 500 {
			httpx.WriteError(w, s.logger, apierror.Validation(nil))
			return
		}
		request.Description = &value
	}
	item, err := s.store.CreateKnowledgeCollection(
		r.Context(), principalFrom(r.Context()), request.Name, request.Description,
	)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, item.Response())
}

func (s *Server) listKnowledgeCollections(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListKnowledgeCollections(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	result := make([]store.KnowledgeCollectionResponse, 0, len(items))
	for _, item := range items {
		result = append(result, item.Response())
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (s *Server) uploadKnowledgeDocument(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxKnowledgeFileBytes+(1<<20))
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	defer file.Close()

	var collectionID *int64
	if raw := strings.TrimSpace(r.FormValue("collection_id")); raw != "" {
		value, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || value <= 0 {
			httpx.WriteError(w, s.logger, apierror.Validation(nil))
			return
		}
		collectionID = &value
	}

	originalName := truncateRunes(filepath.Base(header.Filename), 255)
	extension := strings.ToLower(filepath.Ext(originalName))
	if extension != ".txt" && extension != ".pdf" && extension != ".docx" {
		httpx.WriteError(w, s.logger, apierror.New("DOCUMENT_UNSUPPORTED_TYPE", "仅支持 TXT、PDF 和 DOCX", 422))
		return
	}

	principal := principalFrom(r.Context())
	now := time.Now().UTC()
	relative := filepath.Join(
		"knowledge", principal.TenantID.String(), now.Format("2006"), now.Format("01"),
		uuid.NewString()+extension,
	)
	target, err := s.safeDataPath(relative, "knowledge")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	temp, err := os.CreateTemp(filepath.Dir(target), ".upload-*")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	tempPath := temp.Name()
	committed := false
	defer func() {
		_ = temp.Close()
		if !committed {
			_ = os.Remove(tempPath)
			_ = os.Remove(target)
		}
	}()
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temp, hasher), io.LimitReader(file, s.cfg.MaxKnowledgeFileBytes+1))
	if copyErr != nil {
		httpx.WriteError(w, s.logger, copyErr)
		return
	}
	if written == 0 {
		httpx.WriteError(w, s.logger, apierror.New("EMPTY_FILE", "知识文件不能为空", 422))
		return
	}
	if written > s.cfg.MaxKnowledgeFileBytes {
		httpx.WriteError(w, s.logger, apierror.New("DOCUMENT_TOO_LARGE", "知识文件超过单文件大小限制", 413))
		return
	}
	if err := temp.Sync(); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if err := temp.Close(); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	mimeType, err := validateKnowledgeFile(tempPath, extension, s.cfg.MaxKnowledgeFileBytes)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if err := os.Rename(tempPath, target); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}

	item, err := s.store.AddKnowledgeDocument(r.Context(), principal, store.KnowledgeDocument{
		CollectionID: collectionID,
		OriginalName: originalName,
		StoredPath:   filepath.ToSlash(relative),
		MIMEType:     mimeType,
		Extension:    extension,
		Size:         written,
		SHA256:       hex.EncodeToString(hasher.Sum(nil)),
	})
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	committed = true
	httpx.JSON(w, http.StatusAccepted, item.Response())
}

func (s *Server) listKnowledgeDocuments(w http.ResponseWriter, r *http.Request) {
	limit, err := positiveQueryInt(r.URL.Query().Get("limit"), 20, 100)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	offset := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			httpx.WriteError(w, s.logger, apierror.Validation(nil))
			return
		}
	}
	var collectionID *int64
	if raw := strings.TrimSpace(r.URL.Query().Get("collection_id")); raw != "" {
		value, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || value <= 0 {
			httpx.WriteError(w, s.logger, apierror.Validation(nil))
			return
		}
		collectionID = &value
	}
	items, total, err := s.store.ListKnowledgeDocuments(
		r.Context(), principalFrom(r.Context()), collectionID,
		strings.TrimSpace(r.URL.Query().Get("search")),
		strings.TrimSpace(r.URL.Query().Get("status")), limit, offset,
	)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	result := make([]store.KnowledgeDocumentResponse, 0, len(items))
	for _, item := range items {
		result = append(result, item.Response())
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": result, "total": total})
}

func (s *Server) deleteKnowledgeCollection(w http.ResponseWriter, r *http.Request) {
	id, err := knowledgePathID(r, "collectionID")
	if err == nil {
		err = s.store.DeleteKnowledgeCollection(r.Context(), principalFrom(r.Context()), id)
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) reindexKnowledgeDocument(w http.ResponseWriter, r *http.Request) {
	id, err := knowledgePathID(r, "documentID")
	if err == nil {
		err = s.store.ReindexKnowledgeDocument(r.Context(), principalFrom(r.Context()), id)
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}

func (s *Server) previewKnowledgeDocument(w http.ResponseWriter, r *http.Request) {
	id, err := knowledgePathID(r, "documentID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	preview, err := s.store.KnowledgeDocumentPreview(r.Context(), principalFrom(r.Context()), id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"preview": preview})
}

func (s *Server) getKnowledgeDocument(w http.ResponseWriter, r *http.Request) {
	id, err := knowledgePathID(r, "documentID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	item, err := s.store.GetKnowledgeDocument(r.Context(), principalFrom(r.Context()), id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, item.Response())
}

func (s *Server) downloadKnowledgeDocument(w http.ResponseWriter, r *http.Request) {
	id, err := knowledgePathID(r, "documentID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	item, err := s.store.GetKnowledgeDocument(r.Context(), principalFrom(r.Context()), id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	path, err := s.safeDataPath(item.StoredPath, "knowledge")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		httpx.WriteError(w, s.logger, apierror.New("DOCUMENT_FILE_MISSING", "知识文件缺失", 410))
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

func (s *Server) deleteKnowledgeDocument(w http.ResponseWriter, r *http.Request) {
	id, err := knowledgePathID(r, "documentID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	item, err := s.store.MarkKnowledgeDocumentDeleting(r.Context(), principalFrom(r.Context()), id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	path, pathErr := s.safeDataPath(item.StoredPath, "knowledge")
	if pathErr == nil {
		tombstone := path + ".deleting"
		renameErr := os.Rename(path, tombstone)
		if renameErr != nil && !errors.Is(renameErr, os.ErrNotExist) {
			s.logger.Error("tombstone knowledge document", "document_id", id, "error", renameErr)
			httpx.WriteError(w, s.logger, apierror.New(
				"DOCUMENT_CLEANUP_PENDING", "知识文件已停止检索，磁盘清理等待重试", 503,
			))
			return
		}
		if removeErr := os.Remove(tombstone); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			s.logger.Error("remove knowledge tombstone", "document_id", id, "error", removeErr)
			httpx.WriteError(w, s.logger, apierror.New(
				"DOCUMENT_CLEANUP_PENDING", "知识文件已停止检索，磁盘清理等待重试", 503,
			))
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func knowledgePathID(r *http.Request, name string) (int64, error) {
	raw := strings.TrimSpace(r.PathValue(name))
	if raw == "" {
		// Gin exposes params through URL path values only after its adapter in newer Go.
		raw = strings.TrimSpace(r.URL.Query().Get(name))
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, apierror.Validation(nil)
	}
	return value, nil
}

func validateKnowledgeFile(path, extension string, maxBytes int64) (string, error) {
	switch extension {
	case ".txt":
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer file.Close()
		reader := bufio.NewReader(file)
		var pending []byte
		buffer := make([]byte, 32*1024)
		for {
			n, readErr := reader.Read(buffer)
			if n > 0 {
				pending = append(pending, buffer[:n]...)
				if len(pending) > 4 {
					validateTo := validUTF8PrefixLength(pending)
					if validateTo < 0 || containsNUL(pending[:validateTo]) {
						return "", apierror.New("DOCUMENT_INVALID_SIGNATURE", "TXT 必须是 UTF-8 文本", 422)
					}
					pending = append([]byte(nil), pending[validateTo:]...)
				}
			}
			if errors.Is(readErr, io.EOF) {
				break
			}
			if readErr != nil {
				return "", readErr
			}
		}
		if !utf8.Valid(pending) || containsNUL(pending) {
			return "", apierror.New("DOCUMENT_INVALID_SIGNATURE", "TXT 必须是 UTF-8 文本", 422)
		}
		return "text/plain", nil
	case ".pdf":
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer file.Close()
		header := make([]byte, 5)
		if _, err := io.ReadFull(file, header); err != nil || string(header) != "%PDF-" {
			return "", apierror.New("DOCUMENT_INVALID_SIGNATURE", "PDF 文件签名无效", 422)
		}
		return "application/pdf", nil
	case ".docx":
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		archive, err := zip.OpenReader(path)
		if err != nil {
			return "", apierror.New("DOCUMENT_INVALID_SIGNATURE", "DOCX 容器无效", 422)
		}
		defer archive.Close()
		if len(archive.File) > 2000 {
			return "", apierror.New("DOCUMENT_PARSE_LIMIT", "DOCX 文件结构过于复杂", 422)
		}
		var total uint64
		hasContentTypes := false
		hasDocument := false
		for _, entry := range archive.File {
			cleaned := filepath.Clean(filepath.FromSlash(entry.Name))
			if filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
				return "", apierror.New("DOCUMENT_INVALID_SIGNATURE", "DOCX 包含非法路径", 422)
			}
			total += entry.UncompressedSize64
			if total > uint64(maxBytes*4) || (entry.CompressedSize64 > 0 && entry.UncompressedSize64/entry.CompressedSize64 > 100) {
				return "", apierror.New("DOCUMENT_PARSE_LIMIT", "DOCX 解压规模超过限制", 422)
			}
			hasContentTypes = hasContentTypes || entry.Name == "[Content_Types].xml"
			hasDocument = hasDocument || entry.Name == "word/document.xml"
		}
		if info.Size() <= 0 || !hasContentTypes || !hasDocument {
			return "", apierror.New("DOCUMENT_INVALID_SIGNATURE", "DOCX 结构不完整", 422)
		}
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document", nil
	default:
		return "", apierror.New("DOCUMENT_UNSUPPORTED_TYPE", "不支持的知识文件类型", 422)
	}
}

func validUTF8PrefixLength(value []byte) int {
	for suffix := 0; suffix <= min(3, len(value)); suffix++ {
		end := len(value) - suffix
		if utf8.Valid(value[:end]) {
			return end
		}
	}
	return -1
}

func containsNUL(value []byte) bool {
	for _, item := range value {
		if item == 0 {
			return true
		}
	}
	return false
}
