package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"cortex/backend/internal/blobstore"
	"cortex/backend/internal/config"
	"cortex/backend/internal/store"
	"github.com/jackc/pgx/v5"
)

type item struct {
	kind, id, tenantID, documentID, storedPath, digest string
	size                                               int64
}

func main() {
	apply := flag.Bool("apply", false, "copy, verify, and switch rows to MinIO")
	limit := flag.Int("limit", 0, "maximum objects to process; zero means all")
	flag.Parse()
	cfg, err := config.Load()
	if err != nil {
		fatal("load configuration", err)
	}
	if cfg.MinIOEndpoint == "" {
		fatal("configuration", errors.New("MinIO is required"))
	}
	ctx := context.Background()
	db, err := store.Open(ctx, cfg)
	if err != nil {
		fatal("open database", err)
	}
	defer db.Close()
	local, err := blobstore.NewLocal(cfg.DataDir)
	if err != nil {
		fatal("open local store", err)
	}
	remote, err := blobstore.NewS3(cfg.MinIOEndpoint, cfg.MinIOBucket, cfg.MinIOAccessKey, cfg.MinIOSecretKey, cfg.MinIOSecure)
	if err != nil {
		fatal("open MinIO", err)
	}
	items, err := inventory(ctx, db)
	if err != nil {
		fatal("build inventory", err)
	}
	if *limit > 0 && len(items) > *limit {
		items = items[:*limit]
	}
	fmt.Printf("inventory=%d apply=%t\n", len(items), *apply)
	if !*apply {
		return
	}
	var migrated, failed int
	for _, value := range items {
		if err := migrateOne(ctx, db, local, remote, value); err != nil {
			failed++
			slog.Error("object migration failed", "kind", value.kind, "id", value.id, "code", "OBJECT_MIGRATION_FAILED", "error", err)
			continue
		}
		migrated++
	}
	fmt.Printf("migrated=%d failed=%d\n", migrated, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func inventory(ctx context.Context, db *store.Store) ([]item, error) {
	rows, err := db.AdminPool.Query(ctx, `
SELECT 'attachment',id::text,tenant_id::text,'',stored_path,sha256,size FROM attachments WHERE storage_backend='local' AND deleted_at IS NULL
UNION ALL SELECT 'document',id::text,tenant_id::text,id::text,stored_path,content_hash,size_bytes FROM knowledge_documents WHERE storage_backend='local' AND source_type='upload' AND deleted_at IS NULL
UNION ALL SELECT 'asset',a.id::text,a.tenant_id::text,a.document_id::text,a.stored_path,a.sha256,a.size_bytes FROM knowledge_assets a JOIN knowledge_documents d ON d.tenant_id=a.tenant_id AND d.id=a.document_id WHERE a.storage_backend='local' AND d.deleted_at IS NULL
ORDER BY 1,2`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []item
	for rows.Next() {
		var v item
		if err := rows.Scan(&v.kind, &v.id, &v.tenantID, &v.documentID, &v.storedPath, &v.digest, &v.size); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func migrateOne(ctx context.Context, db *store.Store, local, remote blobstore.BlobStore, value item) error {
	key := objectKey(value)
	reader, info, err := local.Open(ctx, value.storedPath)
	if err != nil {
		return fmt.Errorf("open local object: %w", err)
	}
	defer reader.Close()
	if info.Size != value.size || !strings.EqualFold(info.SHA256, value.digest) {
		return errors.New("local size or checksum mismatch")
	}
	remoteInfo, err := remote.Put(ctx, key, reader, value.size, strings.ToLower(value.digest))
	if err != nil {
		return fmt.Errorf("put MinIO object: %w", err)
	}
	verified, err := verifyRemote(ctx, remote, key, value.size, value.digest)
	if err != nil || !verified {
		_ = remote.Delete(ctx, key, remoteInfo.VersionID)
		if err != nil {
			return err
		}
		return errors.New("remote size or checksum mismatch")
	}
	tx, err := db.AdminPool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		_ = remote.Delete(ctx, key, remoteInfo.VersionID)
		return err
	}
	defer tx.Rollback(ctx)
	var tag pgxTag
	switch value.kind {
	case "attachment":
		tag, err = execTag(ctx, tx, `UPDATE attachments SET storage_backend='minio',object_key=$2,object_version=nullif($3,''),etag=nullif($4,'') WHERE id=$1::bigint AND storage_backend='local' AND stored_path=$5`, value.id, key, remoteInfo.VersionID, remoteInfo.ETag, value.storedPath)
	case "document":
		tag, err = execTag(ctx, tx, `UPDATE knowledge_documents SET storage_backend='minio',object_key=$2,object_version=nullif($3,''),etag=nullif($4,'') WHERE id=$1::uuid AND storage_backend='local' AND stored_path=$5`, value.id, key, remoteInfo.VersionID, remoteInfo.ETag, value.storedPath)
	case "asset":
		tag, err = execTag(ctx, tx, `UPDATE knowledge_assets SET storage_backend='minio',object_key=$2,object_version=nullif($3,''),etag=nullif($4,'') WHERE id=$1::uuid AND storage_backend='local' AND stored_path=$5`, value.id, key, remoteInfo.VersionID, remoteInfo.ETag, value.storedPath)
	}
	if err != nil || tag.rows != 1 {
		_ = remote.Delete(ctx, key, remoteInfo.VersionID)
		if err != nil {
			return err
		}
		return errors.New("row changed during migration")
	}
	if err = tx.Commit(ctx); err != nil {
		_ = remote.Delete(ctx, key, remoteInfo.VersionID)
		return err
	}
	return nil
}

type pgxTag struct{ rows int64 }

func execTag(ctx context.Context, tx pgx.Tx, sql string, args ...any) (pgxTag, error) {
	tag, err := tx.Exec(ctx, sql, args...)
	return pgxTag{rows: tag.RowsAffected()}, err
}

func verifyRemote(ctx context.Context, remote blobstore.BlobStore, key string, size int64, expected string) (bool, error) {
	reader, info, err := remote.Open(ctx, key)
	if err != nil {
		return false, fmt.Errorf("open remote object: %w", err)
	}
	defer reader.Close()
	h := sha256.New()
	n, err := io.Copy(h, reader)
	if err != nil {
		return false, err
	}
	actual := hex.EncodeToString(h.Sum(nil))
	return n == size && info.Size == size && strings.EqualFold(actual, expected), nil
}

func objectKey(v item) string {
	switch v.kind {
	case "attachment":
		return fmt.Sprintf("tenants/%s/attachments/%s/%s", v.tenantID, v.id, v.digest)
	case "document":
		return fmt.Sprintf("tenants/%s/knowledge/%s/%s/source", v.tenantID, v.id, v.digest)
	default:
		return fmt.Sprintf("tenants/%s/knowledge/%s/%s/assets/%s", v.tenantID, v.documentID, v.digest, v.id)
	}
}
func fatal(message string, err error) {
	slog.Error(message, "error", err)
	time.Sleep(10 * time.Millisecond)
	os.Exit(1)
}
