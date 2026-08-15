package store

import (
	"context"
	"errors"
	"strconv"
	"time"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type XHSAuthorization struct {
	ID                 int64      `json:"id"`
	TenantID           uuid.UUID  `json:"-"`
	UserID             int32      `json:"-"`
	Status             string     `json:"status"`
	EncryptedState     []byte     `json:"-"`
	EncryptionNonce    []byte     `json:"-"`
	KeyVersion         int        `json:"-"`
	StateFormat        string     `json:"-"`
	AccountDisplayName *string    `json:"account_display_name"`
	AuthorizedAt       *time.Time `json:"authorized_at"`
	LastVerifiedAt     *time.Time `json:"last_verified_at"`
	ExpiresAt          *time.Time `json:"expires_at"`
	RevokedAt          *time.Time `json:"revoked_at"`
	FailureCode        *string    `json:"failure_code"`
	Version            int        `json:"version"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type XHSAuthAttempt struct {
	ID              uuid.UUID `json:"id"`
	TenantID        uuid.UUID `json:"-"`
	UserID          int32     `json:"-"`
	AuthorizationID int64     `json:"authorization_id"`
	Status          string    `json:"status"`
	QRPath          *string   `json:"-"`
	FailureCode     *string   `json:"failure_code"`
	ExpiresAt       time.Time `json:"expires_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (s *Store) GetXHSAuthorization(
	ctx context.Context, principal domain.Principal,
) (XHSAuthorization, error) {
	var result XHSAuthorization
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		err := scanXHSAuthorization(tx.QueryRow(ctx, `SELECT id,tenant_id,created_by,status,
			encrypted_state,encryption_nonce,key_version,state_format,account_display_name,
			authorized_at,last_verified_at,expires_at,revoked_at,failure_code,version,created_at,updated_at
			FROM xhs_authorizations WHERE tenant_id=$1`, principal.TenantID), &result)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("XHS_AUTH_REQUIRED", "尚未授权小红书账号", 404)
		}
		return err
	})
	return result, err
}

func (s *Store) CreateXHSAuthAttempt(
	ctx context.Context, principal domain.Principal, expiresAt time.Time, keyVersion int,
) (XHSAuthAttempt, error) {
	var result XHSAuthAttempt
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`,
			principal.TenantID); err != nil {
			return err
		}
		err := scanXHSAuthAttempt(tx.QueryRow(ctx, `SELECT id,tenant_id,created_by,authorization_id,
			status,qr_path,failure_code,expires_at,created_at,updated_at
			FROM xhs_auth_attempts
			WHERE tenant_id=$1 AND status IN ('queued','starting','waiting_for_scan','scanned','verification_required')
			AND expires_at>now()
			ORDER BY created_at DESC LIMIT 1`, principal.TenantID), &result)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var authorizationID int64
		if err := tx.QueryRow(ctx, `INSERT INTO xhs_authorizations
			(tenant_id,created_by,status,key_version)
			VALUES($1,$2,'pending',$3)
			ON CONFLICT(tenant_id) DO UPDATE SET status='pending',failure_code=NULL,
				revoked_at=NULL,key_version=EXCLUDED.key_version,version=xhs_authorizations.version+1,
				updated_at=now()
			RETURNING id`, principal.TenantID, principal.UserID, keyVersion).Scan(&authorizationID); err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `INSERT INTO xhs_auth_attempts
			(tenant_id,created_by,authorization_id,expires_at)
			VALUES($1,$2,$3,$4)
			RETURNING id,tenant_id,created_by,authorization_id,status,qr_path,failure_code,
			expires_at,created_at,updated_at`, principal.TenantID, principal.UserID,
			authorizationID, expiresAt).Scan(&result.ID, &result.TenantID, &result.UserID,
			&result.AuthorizationID, &result.Status, &result.QRPath, &result.FailureCode,
			&result.ExpiresAt, &result.CreatedAt, &result.UpdatedAt)
		if err != nil {
			return err
		}
		return auditResource(ctx, tx, principal, "xhs.authorization.start", "xhs_authorization", stringValue(authorizationID))
	})
	return result, err
}

func (s *Store) GetXHSAuthAttempt(
	ctx context.Context, principal domain.Principal, id uuid.UUID,
) (XHSAuthAttempt, error) {
	var result XHSAuthAttempt
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		err := scanXHSAuthAttempt(tx.QueryRow(ctx, `SELECT id,tenant_id,created_by,authorization_id,
			status,qr_path,failure_code,expires_at,created_at,updated_at
			FROM xhs_auth_attempts WHERE tenant_id=$1 AND id=$2`,
			principal.TenantID, id), &result)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("XHS_AUTH_NOT_FOUND", "小红书授权任务不存在", 404)
		}
		return err
	})
	return result, err
}

func (s *Store) ClaimXHSAuthAttempts(
	ctx context.Context, owner string, limit int, lease time.Duration,
) ([]XHSAuthAttempt, error) {
	tx, err := s.AdminPool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `WITH candidates AS (
			SELECT id FROM xhs_auth_attempts
			WHERE ((status='queued') OR
			       (status IN ('starting','waiting_for_scan','scanned','verification_required') AND lease_until<now()))
			  AND expires_at>now()
			ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT $1
		)
		UPDATE xhs_auth_attempts a SET status='starting',lease_owner=$2,
			lease_until=now()+$3::interval,updated_at=now()
		FROM candidates c WHERE a.id=c.id
		RETURNING a.id,a.tenant_id,a.created_by,a.authorization_id,a.status,a.qr_path,
			a.failure_code,a.expires_at,a.created_at,a.updated_at`, limit, owner, lease.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []XHSAuthAttempt
	for rows.Next() {
		var item XHSAuthAttempt
		if err := scanXHSAuthAttempt(rows, &item); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) UpdateXHSAuthAttempt(
	ctx context.Context, principal domain.Principal, id uuid.UUID, status string, qrPath *string, failureCode string,
) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE xhs_auth_attempts SET status=$3,qr_path=$4,
			failure_code=NULLIF($5,''),lease_until=now()+interval '30 seconds',updated_at=now()
			WHERE tenant_id=$1 AND id=$2`, principal.TenantID, id, status, qrPath, failureCode)
		return err
	})
}

func (s *Store) CompleteXHSAuthorization(
	ctx context.Context, principal domain.Principal, attemptID uuid.UUID,
	ciphertext, nonce []byte, keyVersion int, displayName string, expiresAt *time.Time,
) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var authorizationID int64
		if err := tx.QueryRow(ctx, `UPDATE xhs_auth_attempts SET status='authorized',
			qr_path=NULL,lease_owner=NULL,lease_until=NULL,updated_at=now()
			WHERE tenant_id=$1 AND id=$2 RETURNING authorization_id`,
			principal.TenantID, attemptID).Scan(&authorizationID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE xhs_authorizations SET status='authorized',
			encrypted_state=$3,encryption_nonce=$4,key_version=$5,account_display_name=NULLIF($6,''),
			authorized_at=now(),last_verified_at=now(),expires_at=$7,revoked_at=NULL,
			failure_code=NULL,version=version+1,updated_at=now()
			WHERE tenant_id=$1 AND id=$2`, principal.TenantID, authorizationID,
			ciphertext, nonce, keyVersion, displayName, expiresAt); err != nil {
			return err
		}
		return auditResource(ctx, tx, principal, "xhs.authorization.complete", "xhs_authorization", stringValue(authorizationID))
	})
}

func (s *Store) FailXHSAuthAttempt(
	ctx context.Context, principal domain.Principal, attemptID uuid.UUID, code string,
) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var authorizationID int64
		if err := tx.QueryRow(ctx, `UPDATE xhs_auth_attempts SET status='failed',
			failure_code=$3,lease_owner=NULL,lease_until=NULL,updated_at=now()
			WHERE tenant_id=$1 AND id=$2 RETURNING authorization_id`,
			principal.TenantID, attemptID, code).Scan(&authorizationID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE xhs_authorizations SET status='failed',
			failure_code=$3,version=version+1,updated_at=now()
			WHERE tenant_id=$1 AND id=$2`, principal.TenantID, authorizationID, code)
		return err
	})
}

func (s *Store) CancelXHSAuthAttempt(
	ctx context.Context, principal domain.Principal, attemptID uuid.UUID,
) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE xhs_auth_attempts SET status='cancelled',
			qr_path=NULL,lease_owner=NULL,lease_until=NULL,updated_at=now()
			WHERE tenant_id=$1 AND id=$2
			AND status IN ('queued','starting','waiting_for_scan','scanned','verification_required')`,
			principal.TenantID, attemptID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apierror.New("XHS_AUTH_NOT_FOUND", "小红书授权任务不存在或无法取消", 404)
		}
		return auditResource(ctx, tx, principal, "xhs.authorization.cancel", "xhs_auth_attempt", attemptID.String())
	})
}

func (s *Store) RevokeXHSAuthorization(ctx context.Context, principal domain.Principal) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var id int64
		err := tx.QueryRow(ctx, `UPDATE xhs_authorizations SET status='revoked',
			encrypted_state=NULL,encryption_nonce=NULL,revoked_at=now(),lease_owner=NULL,
			lease_until=NULL,version=version+1,updated_at=now()
			WHERE tenant_id=$1 AND status<>'revoked' RETURNING id`, principal.TenantID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("XHS_AUTH_REQUIRED", "尚未授权小红书账号", 404)
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE xhs_auth_attempts SET status='cancelled',
			qr_path=NULL,lease_owner=NULL,lease_until=NULL,updated_at=now()
			WHERE tenant_id=$1 AND status IN ('queued','starting','waiting_for_scan','scanned','verification_required')`,
			principal.TenantID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE research_jobs SET status='cancelled',
			cancel_requested_at=now(),lease_owner=NULL,lease_until=NULL,completed_at=now(),updated_at=now()
			WHERE tenant_id=$1 AND status IN ('queued','collecting','extracting','organizing')`,
			principal.TenantID); err != nil {
			return err
		}
		return auditResource(ctx, tx, principal, "xhs.authorization.revoke", "xhs_authorization", stringValue(id))
	})
}

func (s *Store) MarkXHSAuthorizationVerified(
	ctx context.Context, principal domain.Principal, valid bool,
) error {
	status := "authorized"
	code := ""
	if !valid {
		status = "expired"
		code = "XHS_AUTH_EXPIRED"
	}
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE xhs_authorizations SET status=$2,
			last_verified_at=now(),failure_code=NULLIF($3,''),version=version+1,updated_at=now()
			WHERE tenant_id=$1`, principal.TenantID, status, code)
		return err
	})
}

func (s *Store) AcquireXHSAuthorizationLease(
	ctx context.Context, principal domain.Principal, owner string, lease time.Duration,
) (bool, error) {
	var acquired bool
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE xhs_authorizations
			SET lease_owner=$2,lease_until=now()+$3::interval,updated_at=now()
			WHERE tenant_id=$1 AND status='authorized'
			AND (lease_until IS NULL OR lease_until<now() OR lease_owner=$2)`,
			principal.TenantID, owner, lease.String())
		acquired = err == nil && tag.RowsAffected() == 1
		return err
	})
	return acquired, err
}

func (s *Store) ReleaseXHSAuthorizationLease(
	ctx context.Context, principal domain.Principal, owner string,
) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE xhs_authorizations SET lease_owner=NULL,lease_until=NULL,updated_at=now()
			WHERE tenant_id=$1 AND lease_owner=$2`, principal.TenantID, owner)
		return err
	})
}

func scanXHSAuthorization(scanner knowledgeDocumentScanner, item *XHSAuthorization) error {
	return scanner.Scan(&item.ID, &item.TenantID, &item.UserID, &item.Status,
		&item.EncryptedState, &item.EncryptionNonce, &item.KeyVersion, &item.StateFormat,
		&item.AccountDisplayName, &item.AuthorizedAt, &item.LastVerifiedAt, &item.ExpiresAt,
		&item.RevokedAt, &item.FailureCode, &item.Version, &item.CreatedAt, &item.UpdatedAt)
}

func scanXHSAuthAttempt(scanner knowledgeDocumentScanner, item *XHSAuthAttempt) error {
	return scanner.Scan(&item.ID, &item.TenantID, &item.UserID, &item.AuthorizationID,
		&item.Status, &item.QRPath, &item.FailureCode, &item.ExpiresAt,
		&item.CreatedAt, &item.UpdatedAt)
}

func stringValue(value int64) string {
	return strconv.FormatInt(value, 10)
}
