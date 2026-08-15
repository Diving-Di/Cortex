package store

import (
	"context"
	"errors"
	"time"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/auth"
	"cortex/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) Register(ctx context.Context, username, email, passwordHash string) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM users WHERE username=$1 OR email=$2)`,
			username, email,
		).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return apierror.New("REGISTRATION_CONFLICT", "用户名或邮箱已存在", 400)
		}
		var userID int32
		if err := tx.QueryRow(ctx,
			`INSERT INTO users (username,email,password_hash) VALUES ($1,$2,$3) RETURNING id`,
			username, email, passwordHash,
		).Scan(&userID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO tenants (id,user_id,name) VALUES ($1,$2,$3)`,
			uuid.New(), userID, username+" 的个人空间",
		)
		return err
	})
}

func (s *Store) Login(ctx context.Context, username, password string, ttl time.Duration) (string, string, error) {
	var userID int32
	var storedHash, actualUsername string
	err := s.Pool.QueryRow(ctx,
		`SELECT u.id,u.username,u.password_hash
		 FROM users u JOIN tenants t ON t.user_id=u.id
		 WHERE u.username=$1 AND t.status='active' AND t.deleted_at IS NULL`, username,
	).Scan(&userID, &actualUsername, &storedHash)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !auth.VerifyPassword(password, storedHash)) {
		return "", "", apierror.New("INVALID_CREDENTIALS", "用户名或密码错误", 400)
	}
	if err != nil {
		return "", "", err
	}
	raw, err := auth.NewToken()
	if err != nil {
		return "", "", err
	}
	_, err = s.Pool.Exec(ctx,
		`INSERT INTO auth_tokens (token_hash,user_id,expires_at) VALUES ($1,$2,$3)`,
		auth.HashToken(raw), userID, time.Now().UTC().Add(ttl),
	)
	return raw, actualUsername, err
}

func (s *Store) ResolvePrincipal(ctx context.Context, rawToken string) (domain.Principal, error) {
	var principal domain.Principal
	err := s.Pool.QueryRow(ctx, `
		SELECT tok.id,u.id,u.username,t.id,true,tok.auth_version,t.auth_version,tok.expires_at
        FROM auth_tokens tok
        JOIN users u ON u.id=tok.user_id
        JOIN tenants t ON t.user_id=u.id
		WHERE tok.token_hash=$1 AND tok.revoked_at IS NULL AND tok.expires_at > now()
		  AND t.status='active' AND t.deleted_at IS NULL
        `,
		auth.HashToken(rawToken),
	).Scan(&principal.TokenID, &principal.UserID, &principal.Username, &principal.TenantID, &principal.TenantActive, &principal.TokenVersion, &principal.TenantVersion, &principal.TokenExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Principal{}, apierror.New(
			"AUTHENTICATION_REQUIRED", "Invalid or expired token.", 401,
		)
	}
	if err != nil {
		return domain.Principal{}, err
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE auth_tokens SET last_used_at=now() WHERE id=$1 AND (last_used_at IS NULL OR last_used_at<now()-interval '5 minutes')`, principal.TokenID)
	return principal, nil
}

func (s *Store) RevokeToken(ctx context.Context, tokenID int32) error {
	_, err := s.Pool.Exec(ctx, `UPDATE auth_tokens SET revoked_at=now(),auth_version=auth_version+1 WHERE id=$1`, tokenID)
	return err
}
