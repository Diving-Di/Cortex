package blobstore

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// S3 is the deliberately small S3-compatible client used for the private
// MinIO bucket. It supports only the object operations required by Cortex.
type S3 struct {
	endpoint                             *url.URL
	bucket, accessKey, secretKey, region string
	client                               *http.Client
}

const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func NewS3(endpoint, bucket, accessKey, secretKey string, secure bool) (*S3, error) {
	if !strings.Contains(endpoint, "://") {
		if secure {
			endpoint = "https://" + endpoint
		} else {
			endpoint = "http://" + endpoint
		}
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid MINIO_ENDPOINT")
	}
	if bucket == "" || accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("MinIO bucket and credentials are required")
	}
	return &S3{endpoint: u, bucket: bucket, accessKey: accessKey, secretKey: secretKey, region: "us-east-1", client: &http.Client{Timeout: 30 * time.Second}}, nil
}

func (s *S3) objectURL(key string) *url.URL {
	u := *s.endpoint
	u.Path = path.Join(u.Path, s.bucket, key)
	return &u
}
func hmacSum(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(value))
	return h.Sum(nil)
}
func (s *S3) sign(req *http.Request, payloadHash string, now time.Time) {
	stamp := now.UTC().Format("20060102T150405Z")
	day := now.UTC().Format("20060102")
	req.Header.Set("x-amz-date", stamp)
	req.Header.Set("x-amz-content-sha256", payloadHash)
	canonicalHeaders := "host:" + req.URL.Host + "\n" + "x-amz-content-sha256:" + payloadHash + "\n" + "x-amz-date:" + stamp + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonical := req.Method + "\n" + req.URL.EscapedPath() + "\n" + req.URL.Query().Encode() + "\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + payloadHash
	scope := day + "/" + s.region + "/s3/aws4_request"
	digest := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + hex.EncodeToString(digest[:])
	kDate := hmacSum([]byte("AWS4"+s.secretKey), day)
	kRegion := hmacSum(kDate, s.region)
	kService := hmacSum(kRegion, "s3")
	key := hmacSum(kService, "aws4_request")
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.accessKey+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+hex.EncodeToString(hmacSum(key, toSign)))
}
func (s *S3) do(ctx context.Context, method, key, version string, body io.Reader, size int64, payloadHash string) (*http.Response, error) {
	if key != "" && (strings.HasPrefix(key, "/") || strings.Contains("/"+key+"/", "/../") || strings.Contains(key, "\\")) {
		return nil, fmt.Errorf("invalid object key")
	}
	objectURL := s.objectURL(key)
	if version != "" {
		query := objectURL.Query()
		query.Set("versionId", version)
		objectURL.RawQuery = query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, objectURL.String(), body)
	if err != nil {
		return nil, err
	}
	if size >= 0 {
		req.ContentLength = size
	}
	s.sign(req, payloadHash, time.Now())
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, fmt.Errorf("object store %s failed with status %d", method, resp.StatusCode)
	}
	return resp, nil
}
func (s *S3) Put(ctx context.Context, key string, body io.Reader, size int64, digest string) (ObjectInfo, error) {
	if len(digest) != 64 {
		return ObjectInfo{}, fmt.Errorf("sha256 is required")
	}
	data, err := io.ReadAll(io.LimitReader(body, size+1))
	if err != nil {
		return ObjectInfo{}, err
	}
	if int64(len(data)) != size {
		return ObjectInfo{}, fmt.Errorf("object size mismatch")
	}
	actual := sha256.Sum256(data)
	if hex.EncodeToString(actual[:]) != digest {
		return ObjectInfo{}, fmt.Errorf("object checksum mismatch")
	}
	resp, err := s.do(ctx, http.MethodPut, key, "", bytes.NewReader(data), size, digest)
	if err != nil {
		return ObjectInfo{}, err
	}
	defer resp.Body.Close()
	return ObjectInfo{Key: key, Size: size, SHA256: digest, ETag: strings.Trim(resp.Header.Get("ETag"), "\""), VersionID: resp.Header.Get("x-amz-version-id"), Modified: time.Now().UTC()}, nil
}
func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error) {
	resp, err := s.do(ctx, http.MethodGet, key, "", nil, -1, emptySHA256)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	return resp.Body, responseInfo(key, resp), nil
}
func responseInfo(key string, r *http.Response) ObjectInfo {
	return ObjectInfo{Key: key, Size: r.ContentLength, ETag: strings.Trim(r.Header.Get("ETag"), "\""), VersionID: r.Header.Get("x-amz-version-id"), Modified: time.Now().UTC()}
}
func (s *S3) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	resp, err := s.do(ctx, http.MethodHead, key, "", nil, -1, emptySHA256)
	if err != nil {
		return ObjectInfo{}, err
	}
	defer resp.Body.Close()
	return responseInfo(key, resp), nil
}
func (s *S3) Delete(ctx context.Context, key, version string) error {
	resp, err := s.do(ctx, http.MethodDelete, key, version, nil, -1, emptySHA256)
	if resp != nil {
		resp.Body.Close()
	}
	return err
}
func (s *S3) Ready(ctx context.Context) error {
	resp, err := s.do(ctx, http.MethodHead, "", "", nil, -1, emptySHA256)
	if resp != nil {
		resp.Body.Close()
	}
	return err
}
