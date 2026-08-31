package blobstore

import (
	"context"
	"io"
	"time"
)

type ObjectInfo struct {
	Key       string
	Size      int64
	SHA256    string
	ETag      string
	VersionID string
	Modified  time.Time
}

type BlobStore interface {
	Put(context.Context, string, io.Reader, int64, string) (ObjectInfo, error)
	Open(context.Context, string) (io.ReadCloser, ObjectInfo, error)
	Stat(context.Context, string) (ObjectInfo, error)
	Delete(ctx context.Context, key, version string) error
	Ready(context.Context) error
}
