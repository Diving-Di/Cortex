CREATE TABLE public.ai_flash_event_eligibilities (
    event_id bigint NOT NULL REFERENCES public.ai_flash_events(id) ON DELETE CASCADE,
    tenant_id uuid NOT NULL REFERENCES public.tenants(id) ON DELETE CASCADE,
    streak_days integer NOT NULL CHECK (streak_days > 0),
    available_points bigint NOT NULL CHECK (available_points >= 0),
    snapshot_version uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(event_id, tenant_id)
);

ALTER TABLE public.ai_flash_event_eligibilities ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.ai_flash_event_eligibilities FORCE ROW LEVEL SECURITY;
CREATE POLICY ai_flash_event_eligibilities_tenant_isolation ON public.ai_flash_event_eligibilities
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
GRANT SELECT ON public.ai_flash_event_eligibilities TO cortex_app;

CREATE INDEX idx_notes_created_by ON public.notes(created_by);
CREATE INDEX idx_audit_logs_user_id ON public.audit_logs(user_id);
