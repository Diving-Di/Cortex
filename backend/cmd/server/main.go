package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cortex/backend/internal/blobstore"
	"cortex/backend/internal/config"
	"cortex/backend/internal/rediscoord"
	"cortex/backend/internal/searchindex"
	"cortex/backend/internal/server"
	"cortex/backend/internal/store"
	"cortex/backend/internal/workers"
)

var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load configuration", "error", err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	db, err := store.Open(ctx, cfg)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	localBlobs, err := blobstore.NewLocal(cfg.DataDir)
	if err != nil {
		logger.Error("open local object store", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	var minioBlobs blobstore.BlobStore
	if cfg.StorageBackend == "minio" || (cfg.MinIOEndpoint != "" && cfg.MinIOBucket != "" && cfg.MinIOAccessKey != "" && cfg.MinIOSecretKey != "") {
		minioBlobs, err = blobstore.NewS3(cfg.MinIOEndpoint, cfg.MinIOBucket, cfg.MinIOAccessKey, cfg.MinIOSecretKey, cfg.MinIOSecure)
	}
	if err != nil {
		logger.Error("open object store", "error", err)
		os.Exit(1)
	}
	var blobs blobstore.BlobStore = localBlobs
	if cfg.StorageBackend == "minio" {
		blobs = minioBlobs
	}

	redis, err := rediscoord.New(cfg.RedisURL)
	if err != nil {
		logger.Warn("redis unavailable; distributed coordination will degrade", "code", "REDIS_UNAVAILABLE")
	}
	var search *searchindex.Elasticsearch
	if cfg.RAGRetrievalBackend == "elasticsearch" {
		search = searchindex.New(cfg.ElasticsearchURLs, cfg.ElasticsearchUsername, cfg.ElasticsearchPassword, cfg.ElasticsearchIndexAlias)
	}
	if cfg.RuntimeRole != "api" {
		workers.Run(ctx, cfg, db, blobs, localBlobs, minioBlobs, logger)
		go server.RunScheduler(ctx, cfg, db, logger)
		server.RunAIEventWorkers(ctx, cfg, db, logger)
		server.RunMarketplaceWorker(ctx, db, redis, logger)
	}
	if cfg.RuntimeRole == "worker" {
		logger.Info("worker process started", "version", version)
		<-ctx.Done()
		return
	}

	handler := server.NewWithDependencies(cfg, db, logger, version, server.Dependencies{Blobs: blobs, LocalBlobs: localBlobs, Redis: redis, Search: search})
	httpServer := newHTTPServer(cfg.ListenAddress, handler)

	go func() {
		logger.Info("server starting", "address", cfg.ListenAddress, "version", version)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped unexpectedly", "error", err)
			cancel()
		}
	}()

	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
}

func newHTTPServer(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// Streaming AI responses can legitimately exceed 30 seconds. Each
		// upstream operation owns a context deadline, so a process-wide write
		// deadline would only truncate valid SSE streams.
		WriteTimeout: 0,
		IdleTimeout:  90 * time.Second,
	}
}
