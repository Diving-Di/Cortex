package main

import (
	"context"
	"cortex/backend/internal/blobstore"
	"cortex/backend/internal/config"
	"cortex/backend/internal/server"
	"cortex/backend/internal/store"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
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
	local, _ := blobstore.NewLocal(cfg.DataDir)
	server.RunKnowledgeIndexer(ctx, cfg, db, blobs, local, slog.Default())
	<-ctx.Done()
}
