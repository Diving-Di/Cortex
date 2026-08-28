package store

import (
	"context"

	"cortex/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func tenantSummary(ctx context.Context, tx pgx.Tx, principal domain.Principal) (domain.TenantSummary, error) {
	var summary domain.TenantSummary
	err := tx.QueryRow(ctx, `
        SELECT t.id::text,t.name,t.status,t.note_quota,
            (SELECT count(*) FROM notes n WHERE n.tenant_id=t.id AND n.deleted_at IS NULL),
            t.attachment_quota_bytes,t.ai_token_quota,
            COALESCE((SELECT sum(input_tokens+output_tokens) FROM ai_usage_records a WHERE a.tenant_id=t.id),0)
        FROM tenants t WHERE t.id=$1`, principal.TenantID,
	).Scan(
		&summary.ID, &summary.Name, &summary.Status, &summary.NoteQuota,
		&summary.NoteCount, &summary.AttachmentQuotaBytes, &summary.AITokenQuota,
		&summary.AITokensUsed,
	)
	return summary, err
}

func (s *Store) GetTenant(ctx context.Context, principal domain.Principal) (domain.TenantSummary, error) {
	var result domain.TenantSummary
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var err error
		result, err = tenantSummary(ctx, tx, principal)
		return err
	})
	return result, err
}

func (s *Store) UpdateTenant(ctx context.Context, principal domain.Principal, name string) (domain.TenantSummary, error) {
	var result domain.TenantSummary
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE tenants SET name=$1,updated_at=now() WHERE id=$2`, name, principal.TenantID); err != nil {
			return err
		}
		if err := auditResource(ctx, tx, principal, "tenant.update", "tenant", principal.TenantID.String()); err != nil {
			return err
		}
		var err error
		result, err = tenantSummary(ctx, tx, principal)
		return err
	})
	return result, err
}

func (s *Store) DeleteTenant(ctx context.Context, principal domain.Principal) ([]uuid.UUID, error) {
	var publicIDs []uuid.UUID
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		if err := auditResource(ctx, tx, principal, "tenant.soft_delete", "tenant", principal.TenantID.String()); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT p.public_template_id FROM published_template_snapshots p JOIN template_publications tp ON tp.id=p.source_publication_id WHERE tp.tenant_id=$1 AND p.status='published'`, principal.TenantID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			publicIDs = append(publicIDs, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if _, err := tx.Exec(ctx, `UPDATE published_template_snapshots SET status='withdrawn',withdrawn_at=now()
			WHERE status='published' AND source_publication_id IN
			(SELECT id FROM template_publications WHERE tenant_id=$1)`, principal.TenantID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE writing_templates SET status='withdrawn',updated_at=now()
			WHERE tenant_id=$1 AND status='published'`, principal.TenantID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE template_publications SET status='withdrawn',withdrawn_at=now() WHERE tenant_id=$1 AND status='published'`, principal.TenantID); err != nil {
			return err
		}
		for _, id := range publicIDs {
			if _, err := tx.Exec(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type) VALUES($1,'template',$2,'template.withdrawn')`, uuid.New(), id.String()); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE tenants SET status='deleted',deleted_at=now(),updated_at=now() WHERE id=$1`, principal.TenantID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE auth_tokens SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, principal.UserID)
		return err
	})
	return publicIDs, err
}

func auditResource(
	ctx context.Context,
	tx pgx.Tx,
	principal domain.Principal,
	action string,
	resourceType string,
	resourceID string,
) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_logs
        (tenant_id,user_id,action,resource_type,resource_id) VALUES ($1,$2,$3,$4,$5)`,
		principal.TenantID, principal.UserID, action, resourceType, resourceID,
	)
	return err
}
