package store

import (
    "context"

    "diary-listener/backend/internal/apierror"
    "diary-listener/backend/internal/domain"
    "github.com/jackc/pgx/v5"
)

type TenantSummary struct {
    ID                   string `json:"id"`
    Name                 string `json:"name"`
    Status               string `json:"status"`
    NoteQuota            int64  `json:"note_quota"`
    NoteCount            int64  `json:"note_count"`
    AttachmentQuotaBytes int64  `json:"attachment_quota_bytes"`
    AITokenQuota         int64  `json:"ai_token_quota"`
    AITokensUsed         int64  `json:"ai_tokens_used"`
}

func tenantSummary(ctx context.Context, tx pgx.Tx, principal domain.Principal) (TenantSummary, error) {
    var summary TenantSummary
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

func (s *Store) GetTenant(ctx context.Context, principal domain.Principal) (TenantSummary, error) {
    var result TenantSummary
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

func (s *Store) UpdateTenant(ctx context.Context, principal domain.Principal, name string) (TenantSummary, error) {
    var result TenantSummary
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

func (s *Store) DeleteTenant(ctx context.Context, principal domain.Principal) error {
    return s.WithTx(ctx, func(tx pgx.Tx) error {
        if err := setTenant(ctx, tx, principal); err != nil {
            return err
        }
        if err := auditResource(ctx, tx, principal, "tenant.soft_delete", "tenant", principal.TenantID.String()); err != nil {
            return err
        }
        _, err := tx.Exec(ctx, `UPDATE tenants SET status='deleted',deleted_at=now(),updated_at=now() WHERE id=$1`, principal.TenantID)
        return err
    })
}

func (s *Store) RestoreTenant(ctx context.Context, principal domain.Principal) (TenantSummary, error) {
    var result TenantSummary
    err := s.WithTx(ctx, func(tx pgx.Tx) error {
        command, err := tx.Exec(ctx, `
            UPDATE tenants SET status='active',deleted_at=NULL,updated_at=now()
            WHERE id=$1 AND user_id=$2 AND status='deleted'`, principal.TenantID, principal.UserID,
        )
        if err != nil {
            return err
        }
        if command.RowsAffected() == 0 {
            return apierror.New("TENANT_NOT_DELETED", "个人空间不处于可恢复状态", 409)
        }
        if err := setTenant(ctx, tx, principal); err != nil {
            return err
        }
        if err := auditResource(ctx, tx, principal, "tenant.restore", "tenant", principal.TenantID.String()); err != nil {
            return err
        }
        result, err = tenantSummary(ctx, tx, principal)
        return err
    })
    return result, err
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
