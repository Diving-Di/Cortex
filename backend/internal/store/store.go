package store

import (
	"context"
	"fmt"
	"sync"
	"time"

	"cortex/backend/internal/config"
	"cortex/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	Pool          *pgxpool.Pool
	AuthPool      *pgxpool.Pool
	AdminPool     *pgxpool.Pool
	authTouches   chan int32
	authTouchStop chan struct{}
	authTouchDone chan struct{}
	closeOnce     sync.Once
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
	authConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("parse auth DATABASE_URL: %w", err)
	}
	authConfig.MaxConns = cfg.AuthPoolSize
	authConfig.ConnConfig.RuntimeParams["statement_timeout"] = fmt.Sprintf("%d", cfg.StatementTimeout.Milliseconds())
	authPool, err := pgxpool.NewWithConfig(ctx, authConfig)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("create auth database pool: %w", err)
	}
	if err := authPool.Ping(ctx); err != nil {
		pool.Close()
		authPool.Close()
		return nil, fmt.Errorf("ping auth database pool: %w", err)
	}
	s := &Store{Pool: pool, AuthPool: authPool, authTouches: make(chan int32, 4096), authTouchStop: make(chan struct{}), authTouchDone: make(chan struct{})}
	if cfg.RuntimeRole == "api" {
		go s.runAuthTouches()
		return s, nil
	}
	adminConfig, err := pgxpool.ParseConfig(cfg.MigrationDatabaseURL)
	if err != nil {
		pool.Close()
		authPool.Close()
		return nil, fmt.Errorf("parse MIGRATION_DATABASE_URL: %w", err)
	}
	adminConfig.MaxConns = 2
	adminPool, err := pgxpool.NewWithConfig(ctx, adminConfig)
	if err != nil {
		pool.Close()
		authPool.Close()
		return nil, fmt.Errorf("create migration database pool: %w", err)
	}
	if err := adminPool.Ping(ctx); err != nil {
		pool.Close()
		authPool.Close()
		adminPool.Close()
		return nil, fmt.Errorf("ping migration database: %w", err)
	}
	s.AdminPool = adminPool
	go s.runAuthTouches()
	return s, nil
}

func (s *Store) Close() {
	s.closeOnce.Do(func() {
		close(s.authTouchStop)
		<-s.authTouchDone
		s.Pool.Close()
		if s.AuthPool != nil {
			s.AuthPool.Close()
		}
		if s.AdminPool != nil {
			s.AdminPool.Close()
		}
	})
}

func (s *Store) touchAuthToken(id int32) {
	if s.authTouches == nil {
		return
	}
	select {
	case s.authTouches <- id:
	default:
	}
}

func (s *Store) runAuthTouches() {
	defer close(s.authTouchDone)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	pending := make(map[int32]struct{}, 256)
	flush := func() {
		if len(pending) == 0 {
			return
		}
		ids := make([]int32, 0, len(pending))
		for id := range pending {
			ids = append(ids, id)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, _ = s.authPool().Exec(ctx, `UPDATE auth_tokens SET last_used_at=now() WHERE id=ANY($1) AND (last_used_at IS NULL OR last_used_at<now()-interval '5 minutes')`, ids)
		cancel()
		clear(pending)
	}
	for {
		select {
		case id := <-s.authTouches:
			pending[id] = struct{}{}
		case <-ticker.C:
			flush()
		case <-s.authTouchStop:
			flush()
			return
		}
	}
}

func (s *Store) Ping(ctx context.Context) error { return s.Pool.Ping(ctx) }

func (s *Store) authPool() *pgxpool.Pool {
	if s.AuthPool != nil {
		return s.AuthPool
	}
	return s.Pool
}

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

// WithPrincipalTx is the mandatory transaction boundary for tenant business
// data. RLS context is transaction-local and cannot leak through the pool.
func (s *Store) WithPrincipalTx(ctx context.Context, principal domain.Principal, fn func(pgx.Tx) error) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		return fn(tx)
	})
}
