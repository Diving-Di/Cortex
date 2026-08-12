ALTER TABLE public.auth_tokens
    ADD COLUMN auth_version bigint NOT NULL DEFAULT 1;

ALTER TABLE public.tenants
    ADD COLUMN auth_version bigint NOT NULL DEFAULT 1;

ALTER TABLE public.ai_flash_claims
    ADD COLUMN reservation_token uuid,
    ADD COLUMN reservation_version text;

CREATE UNIQUE INDEX ai_flash_claims_reservation_token_unique
    ON public.ai_flash_claims(reservation_token)
    WHERE reservation_token IS NOT NULL;

CREATE TABLE public.ai_event_reservations (
    token uuid PRIMARY KEY,
    event_id bigint NOT NULL REFERENCES public.ai_flash_events(id) ON DELETE CASCADE,
    tenant_id uuid NOT NULL REFERENCES public.tenants(id) ON DELETE CASCADE,
    projection_version text,
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','confirmed','compensated')),
    created_at timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz,
    UNIQUE(event_id, tenant_id)
);

CREATE INDEX ai_event_reservations_pending_idx
    ON public.ai_event_reservations(created_at)
    WHERE state='pending';

ALTER TABLE public.ai_event_reservations ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.ai_event_reservations FORCE ROW LEVEL SECURITY;
CREATE POLICY ai_event_reservations_tenant_policy ON public.ai_event_reservations
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.ai_event_reservations TO diary_app;
