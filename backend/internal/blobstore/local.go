package blobstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Local struct{ root string }

func NewLocal(root string) (*Local, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Local{root: abs}, nil
}

func (s *Local) path(key string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(key))
	if filepath.IsAbs(clean) || clean == "." || clean == ".." {
		return "", fmt.Errorf("invalid object key")
	}
	target := filepath.Join(s.root, clean)
	rel, err := filepath.Rel(s.root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid object key")
	}
	return target, nil
}

func (s *Local) Put(ctx context.Context, key string, body io.Reader, size int64, expected string) (ObjectInfo, error) {
	target, err := s.path(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	if err = os.MkdirAll(filepath.Dir(target), 0750); err != nil {
		return ObjectInfo{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".upload-*")
	if err != nil {
		return ObjectInfo{}, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(body, size+1))
	closeErr := tmp.Close()
	actual := hex.EncodeToString(h.Sum(nil))
	if copyErr != nil {
		return ObjectInfo{}, copyErr
	}
	if closeErr != nil {
		return ObjectInfo{}, closeErr
	}
	if n != size || (expected != "" && actual != expected) {
		return ObjectInfo{}, fmt.Errorf("object checksum or size mismatch")
	}
	if err = os.Rename(tmpName, target); err != nil {
		return ObjectInfo{}, err
	}
	return s.Stat(ctx, key)
}

func (s *Local) Open(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error) {
	info, err := s.Stat(ctx, key)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	p, _ := s.path(key)
	f, err := os.Open(p)
	return f, info, err
}
func (s *Local) Stat(_ context.Context, key string) (ObjectInfo, error) {
	p, err := s.path(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	st, err := os.Stat(p)
	if err != nil {
		return ObjectInfo{}, err
	}
	f, err := os.Open(p)
	if err != nil {
		return ObjectInfo{}, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return ObjectInfo{}, err
	}
	digest := hex.EncodeToString(h.Sum(nil))
	return ObjectInfo{Key: key, Size: st.Size(), SHA256: digest, ETag: digest, Modified: st.ModTime()}, nil
}
func (s *Local) Delete(_ context.Context, key string) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
func (s *Local) Ready(_ context.Context) error { return os.MkdirAll(s.root, 0750) }
