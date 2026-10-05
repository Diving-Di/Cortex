CREATE TABLE public.report_generation_jobs (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES public.tenants(id) ON DELETE CASCADE,
    created_by integer NOT NULL REFERENCES public.users(id),
    request_id uuid NOT NULL,
    report_type varchar(16) NOT NULL CHECK (report_type IN ('daily','weekly','monthly')),
    anchor_date date NOT NULL,
    status varchar(16) NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','success','failed','cancelled')),
    stage varchar(24) NOT NULL DEFAULT 'queued',
    sources jsonb NOT NULL DEFAULT '[]',
    source_versions jsonb NOT NULL DEFAULT '{}',
    target_updated_at timestamptz,
    content text NOT NULL DEFAULT '',
    failure_code varchar(80),
    lease_owner uuid,
    lease_until timestamptz,
    dispatch_at timestamptz NOT NULL DEFAULT now(),
    scheduled_task_id integer REFERENCES public.scheduled_report_tasks(id) ON DELETE SET NULL,
    scheduled_run_id bigint REFERENCES public.scheduled_report_runs(id) ON DELETE SET NULL,
    confirmed_note_id integer,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id,request_id),
    FOREIGN KEY (tenant_id,confirmed_note_id) REFERENCES public.notes(tenant_id,id) ON DELETE SET NULL (confirmed_note_id)
);
CREATE INDEX report_jobs_dispatch ON public.report_generation_jobs(dispatch_at) WHERE status='queued';
CREATE INDEX report_jobs_tenant ON public.report_generation_jobs(tenant_id,created_at DESC);
CREATE UNIQUE INDEX report_jobs_active_schedule ON public.report_generation_jobs(scheduled_task_id) WHERE scheduled_task_id IS NOT NULL AND status IN ('queued','running');
ALTER TABLE public.report_generation_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.report_generation_jobs FORCE ROW LEVEL SECURITY;
CREATE POLICY report_jobs_tenant_isolation ON public.report_generation_jobs
    USING (tenant_id=NULLIF(current_setting('app.current_tenant_id',true),'')::uuid)
    WITH CHECK (tenant_id=NULLIF(current_setting('app.current_tenant_id',true),'')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON public.report_generation_jobs TO cortex_app;
