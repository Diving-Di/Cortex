ALTER TABLE public.knowledge_index_jobs
    DROP CONSTRAINT knowledge_index_jobs_stage_check;

ALTER TABLE public.knowledge_index_jobs
    ADD CONSTRAINT knowledge_index_jobs_stage_check
        CHECK (stage IN ('queued','loading','parsing','parsed','embedding','persisting','completed','failed'));

CREATE TABLE public.knowledge_index_artifacts (
    tenant_id uuid NOT NULL,
    job_id bigint NOT NULL,
    document_id uuid NOT NULL,
    target_index_version integer NOT NULL,
    chunks jsonb NOT NULL,
    child_count integer NOT NULL CHECK (child_count > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, job_id),
    FOREIGN KEY (job_id) REFERENCES public.knowledge_index_jobs(id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, document_id) REFERENCES public.knowledge_documents(tenant_id, id) ON DELETE CASCADE,
    CHECK (jsonb_typeof(chunks) = 'array')
);

ALTER TABLE public.knowledge_index_artifacts ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.knowledge_index_artifacts FORCE ROW LEVEL SECURITY;
CREATE POLICY knowledge_index_artifacts_tenant_isolation ON public.knowledge_index_artifacts
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON public.knowledge_index_artifacts TO cortex_app;

