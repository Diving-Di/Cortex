CREATE TABLE public.ai_flash_event_inventory_slots (
    event_id bigint NOT NULL REFERENCES public.ai_flash_events(id) ON DELETE CASCADE,
    slot_number integer NOT NULL CHECK (slot_number > 0),
    tenant_id uuid REFERENCES public.tenants(id) ON DELETE CASCADE,
    claim_id bigint UNIQUE REFERENCES public.ai_flash_claims(id) ON DELETE CASCADE,
    claimed_at timestamptz,
    PRIMARY KEY (event_id, slot_number),
    CHECK ((tenant_id IS NULL AND claim_id IS NULL AND claimed_at IS NULL)
        OR (tenant_id IS NOT NULL AND claim_id IS NOT NULL AND claimed_at IS NOT NULL))
);

CREATE UNIQUE INDEX ai_flash_event_inventory_slots_tenant_unique
    ON public.ai_flash_event_inventory_slots(event_id, tenant_id)
    WHERE tenant_id IS NOT NULL;
CREATE INDEX ai_flash_event_inventory_slots_available
    ON public.ai_flash_event_inventory_slots(event_id, slot_number)
    WHERE tenant_id IS NULL;

ALTER TABLE public.ai_flash_event_inventory_slots ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.ai_flash_event_inventory_slots FORCE ROW LEVEL SECURITY;
CREATE POLICY ai_flash_event_inventory_slots_tenant_isolation
    ON public.ai_flash_event_inventory_slots
    USING (tenant_id IS NULL OR tenant_id = current_setting('app.current_tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant_id', true)::uuid);

GRANT SELECT,UPDATE ON public.ai_flash_event_inventory_slots TO cortex_app;

INSERT INTO public.ai_flash_event_inventory_slots(event_id, slot_number)
SELECT e.id, generate_series(1, e.total_slots)
FROM public.ai_flash_events e
ON CONFLICT DO NOTHING;

UPDATE public.ai_flash_events e
SET claimed_slots = (SELECT count(*)::integer FROM public.ai_flash_claims c WHERE c.event_id=e.id)
WHERE e.claimed_slots <> (SELECT count(*)::integer FROM public.ai_flash_claims c WHERE c.event_id=e.id);
