package store

import (
    "context"
    "fmt"

    "diary-listener/backend/internal/config"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
    Pool      *pgxpool.Pool
    AdminPool *pgxpool.Pool
}

func Open(ctx context.Context, cfg config.Config) (*Store, error) {
    poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
    if err != nil {
        return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
    }
    poolConfig.MaxConns = cfg.PoolSize
    poolConfig.ConnConfig.RuntimeParams["statement_timeout"] = fmt.Sprintf("%d", cfg.StatementTimeout.Milliseconds())
    pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
    if err != nil {
        return nil, fmt.Errorf("create database pool: %w", err)
    }
    if err := pool.Ping(ctx); err != nil {
        pool.Close()
        return nil, fmt.Errorf("ping database: %w", err)
    }
    adminConfig, err := pgxpool.ParseConfig(cfg.MigrationDatabaseURL)
    if err != nil {
        pool.Close()
        return nil, fmt.Errorf("parse MIGRATION_DATABASE_URL: %w", err)
    }
    adminConfig.MaxConns = 2
    adminPool, err := pgxpool.NewWithConfig(ctx, adminConfig)
    if err != nil {
        pool.Close()
        return nil, fmt.Errorf("create migration database pool: %w", err)
    }
    if err := adminPool.Ping(ctx); err != nil {
        pool.Close()
        adminPool.Close()
        return nil, fmt.Errorf("ping migration database: %w", err)
    }
    return &Store{Pool: pool, AdminPool: adminPool}, nil
}

func (s *Store) Close() {
    s.Pool.Close()
    s.AdminPool.Close()
}

func (s *Store) Ping(ctx context.Context) error { return s.Pool.Ping(ctx) }

func (s *Store) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
    tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        return err
    }
    defer func() { _ = tx.Rollback(ctx) }()
    if err := fn(tx); err != nil {
        return err
    }
    return tx.Commit(ctx)
}
