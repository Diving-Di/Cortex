DROP POLICY IF EXISTS ai_flash_event_eligibilities_tenant_isolation ON public.ai_flash_event_eligibilities;
CREATE POLICY ai_flash_event_eligibilities_tenant_isolation ON public.ai_flash_event_eligibilities
    USING (tenant_id = current_setting('app.current_tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.current_tenant_id', true)::uuid);
