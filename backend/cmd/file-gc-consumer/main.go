package main

import (
	"context"
	"cortex/backend/internal/blobstore"
	"cortex/backend/internal/config"
	"cortex/backend/internal/store"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load configuration", "error", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	db, err := store.Open(ctx, cfg)
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	var blobs blobstore.BlobStore
	if cfg.StorageBackend == "minio" {
		blobs, err = blobstore.NewS3(cfg.MinIOEndpoint, cfg.MinIOBucket, cfg.MinIOAccessKey, cfg.MinIOSecretKey, cfg.MinIOSecure)
	} else {
		blobs, err = blobstore.NewLocal(cfg.DataDir)
	}
	if err != nil {
		slog.Error("open object store", "error", err)
		os.Exit(1)
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		job, claimErr := db.ClaimObjectGC(ctx)
		if claimErr != nil {
			slog.Error("claim object gc", "code", "OBJECT_GC_CLAIM_FAILED")
		} else if job != nil {
			deleteErr := blobs.Delete(ctx, job.Key)
			_ = db.FinishObjectGC(ctx, job.ID, deleteErr == nil)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
