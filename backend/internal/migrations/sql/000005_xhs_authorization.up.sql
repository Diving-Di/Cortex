CREATE TABLE public.xhs_authorizations (
    id bigserial PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES public.tenants(id) ON DELETE CASCADE,
    created_by integer NOT NULL,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','authorized','expired','revoked','failed')),
    encrypted_state bytea,
    encryption_nonce bytea,
    key_version integer NOT NULL,
    state_format text NOT NULL DEFAULT 'chromedp-v1',
    account_display_name text,
    authorized_at timestamptz,
    last_verified_at timestamptz,
    expires_at timestamptz,
    revoked_at timestamptz,
    failure_code text,
    lease_owner text,
    lease_until timestamptz,
    version integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, created_by) REFERENCES public.tenants(id, user_id) ON DELETE CASCADE,
    CHECK ((encrypted_state IS NULL) = (encryption_nonce IS NULL))
);

CREATE TABLE public.xhs_auth_attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES public.tenants(id) ON DELETE CASCADE,
    created_by integer NOT NULL,
    authorization_id bigint NOT NULL,
    status text NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued','starting','waiting_for_scan','scanned',
                          'verification_required','authorized','failed','cancelled','expired')),
    qr_path text,
    failure_code text,
    lease_owner text,
    lease_until timestamptz,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, created_by) REFERENCES public.tenants(id, user_id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, authorization_id)
        REFERENCES public.xhs_authorizations(tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX ix_xhs_auth_attempts_claim
    ON public.xhs_auth_attempts(status, expires_at, lease_until, created_at)
    WHERE status IN ('queued','starting','waiting_for_scan','scanned','verification_required');
CREATE INDEX ix_xhs_auth_attempts_tenant
    ON public.xhs_auth_attempts(tenant_id, created_at DESC);
CREATE INDEX ix_xhs_authorizations_lease
    ON public.xhs_authorizations(lease_until)
    WHERE status='authorized';

ALTER TABLE public.xhs_authorizations ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.xhs_authorizations FORCE ROW LEVEL SECURITY;
ALTER TABLE public.xhs_auth_attempts ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.xhs_auth_attempts FORCE ROW LEVEL SECURITY;

CREATE POLICY xhs_authorizations_tenant_isolation ON public.xhs_authorizations
    USING (tenant_id=NULLIF(current_setting('app.current_tenant_id',true),'')::uuid)
    WITH CHECK (tenant_id=NULLIF(current_setting('app.current_tenant_id',true),'')::uuid);
CREATE POLICY xhs_auth_attempts_tenant_isolation ON public.xhs_auth_attempts
    USING (tenant_id=NULLIF(current_setting('app.current_tenant_id',true),'')::uuid)
    WITH CHECK (tenant_id=NULLIF(current_setting('app.current_tenant_id',true),'')::uuid);

GRANT SELECT,INSERT,UPDATE,DELETE ON public.xhs_authorizations TO diary_app;
GRANT SELECT,INSERT,UPDATE,DELETE ON public.xhs_auth_attempts TO diary_app;
GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO diary_app;
