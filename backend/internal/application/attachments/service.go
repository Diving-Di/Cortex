package attachments

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/blobstore"
	"cortex/backend/internal/domain"
	"github.com/google/uuid"
)

type Repository interface {
	AddAttachment(context.Context, domain.Principal, domain.Attachment) (domain.Attachment, error)
	GetAttachment(context.Context, domain.Principal, int32) (domain.Attachment, error)
	ListAttachments(context.Context, domain.Principal, int32) ([]domain.Attachment, error)
	DeleteAttachment(context.Context, domain.Principal, int32) error
}

type Service struct {
	repository     Repository
	blobs          blobstore.BlobStore
	localBlobs     blobstore.BlobStore
	storageBackend string
}

type Download struct {
	Item domain.Attachment
	Body io.ReadCloser
	Size int64
}

func NewService(repository Repository, blobs, localBlobs blobstore.BlobStore, storageBackend string) *Service {
	return &Service{repository: repository, blobs: blobs, localBlobs: localBlobs, storageBackend: storageBackend}
}

func (s *Service) Upload(ctx context.Context, p domain.Principal, noteID int32, filename string, data []byte) (domain.Attachment, error) {
	extension, mimeType, err := Validate(filename, data)
	if err != nil {
		return domain.Attachment{}, err
	}
	digest := sha256.Sum256(data)
	digestText := hex.EncodeToString(digest[:])
	key := filepath.ToSlash(filepath.Join("tenants", p.TenantID.String(), "attachments", uuid.NewString(), digestText+extension))
	object, err := s.blobs.Put(ctx, key, bytes.NewReader(data), int64(len(data)), digestText)
	if err != nil {
		return domain.Attachment{}, apierror.New("ATTACHMENT_STORAGE_UNAVAILABLE", "附件存储暂不可用", 503)
	}
	item, err := s.repository.AddAttachment(ctx, p, domain.Attachment{NoteID: noteID,
		OriginalName: truncate(filepath.Base(filename), 255), StoredPath: key, StorageBackend: s.storageBackend,
		ObjectKey: key, ObjectVersion: object.VersionID, ETag: object.ETag, MIMEType: mimeType,
		Size: int64(len(data)), SHA256: digestText})
	if err != nil {
		_ = s.blobs.Delete(ctx, key)
	}
	return item, err
}

func (s *Service) List(ctx context.Context, p domain.Principal, noteID int32) ([]domain.Attachment, error) {
	return s.repository.ListAttachments(ctx, p, noteID)
}

func (s *Service) Download(ctx context.Context, p domain.Principal, id int32) (Download, error) {
	item, err := s.repository.GetAttachment(ctx, p, id)
	if err != nil {
		return Download{}, err
	}
	key := item.ObjectKey
	if key == "" {
		key = item.StoredPath
	}
	backend := s.blobs
	if item.StorageBackend == "local" {
		backend = s.localBlobs
	}
	body, info, err := backend.Open(ctx, key)
	if err != nil {
		return Download{}, apierror.New("ATTACHMENT_FILE_MISSING", "附件文件缺失", 410)
	}
	return Download{Item: item, Body: body, Size: info.Size}, nil
}

func (s *Service) Delete(ctx context.Context, p domain.Principal, id int32) error {
	return s.repository.DeleteAttachment(ctx, p, id)
}

func Validate(filename string, data []byte) (string, string, error) {
	if len(data) == 0 {
		return "", "", apierror.New("EMPTY_FILE", "附件不能为空", 422)
	}
	extension := strings.ToLower(filepath.Ext(filename))
	signatures := map[string]struct {
		mime string
		data []byte
	}{
		".png": {"image/png", []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}},
		".jpg": {"image/jpeg", []byte{0xff, 0xd8, 0xff}}, ".jpeg": {"image/jpeg", []byte{0xff, 0xd8, 0xff}},
		".pdf": {"application/pdf", []byte("%PDF-")},
	}
	if signature, ok := signatures[extension]; ok {
		if !bytes.HasPrefix(data, signature.data) {
			return "", "", apierror.New("INVALID_FILE_SIGNATURE", "文件内容与扩展名不匹配", 422)
		}
		return extension, signature.mime, nil
	}
	if mimeType, ok := map[string]string{".txt": "text/plain", ".md": "text/markdown"}[extension]; ok {
		if !utf8.Valid(data) {
			return "", "", apierror.New("INVALID_FILE_SIGNATURE", "文本附件必须为 UTF-8", 422)
		}
		return extension, mimeType, nil
	}
	return "", "", apierror.New("UNSUPPORTED_FILE_TYPE", "不支持的附件类型", 422)
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}
