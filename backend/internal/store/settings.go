package store

import (
	"context"
	"errors"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

// UserPreferences holds user-level settings. Recipe-specific fields
// (dietary restrictions and timezone) were removed together with the
// recipe subsystem; only marketplace personalization remains.
type UserPreferences struct {
	TenantID                   string
	UserID                     int32
	Version                    int
	MarketplacePersonalization bool
}

func (s *Store) GetUserPreferences(ctx context.Context, principal domain.Principal) (UserPreferences, error) {
	p := UserPreferences{
		TenantID:                   principal.TenantID.String(),
		UserID:                     principal.UserID,
		Version:                    0,
		MarketplacePersonalization: true,
	}
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT tenant_id::text,user_id,version,marketplace_personalization
			FROM user_preferences WHERE tenant_id=$1 AND user_id=$2`,
			principal.TenantID, principal.UserID).
			Scan(&p.TenantID, &p.UserID, &p.Version, &p.MarketplacePersonalization)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	return p, err
}

func (s *Store) UpdateUserPreferences(ctx context.Context, principal domain.Principal, personalization bool, version int) (UserPreferences, error) {
	var p UserPreferences
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `INSERT INTO user_preferences
				(tenant_id,user_id,marketplace_personalization,version,created_at,updated_at)
			VALUES ($1,$2,$3,1,now(),now())
			ON CONFLICT (tenant_id,user_id) DO UPDATE SET
				marketplace_personalization=EXCLUDED.marketplace_personalization,
				version=user_preferences.version+1,
				updated_at=now()
			WHERE user_preferences.version=$4`,
			principal.TenantID, principal.UserID, personalization, version)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apierror.New("VERSION_CONFLICT", "偏好设置已在其他设备更新", 409)
		}
		return tx.QueryRow(ctx, `SELECT tenant_id::text,user_id,version,marketplace_personalization
			FROM user_preferences WHERE tenant_id=$1 AND user_id=$2`,
			principal.TenantID, principal.UserID).
			Scan(&p.TenantID, &p.UserID, &p.Version, &p.MarketplacePersonalization)
	})
	return p, err
}
