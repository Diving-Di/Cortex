DROP POLICY IF EXISTS ai_event_reservations_tenant_policy ON public.ai_event_reservations;
CREATE POLICY ai_event_reservations_tenant_policy ON public.ai_event_reservations
    USING (tenant_id = nullif(current_setting('app.current_tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.current_tenant_id', true), '')::uuid);
