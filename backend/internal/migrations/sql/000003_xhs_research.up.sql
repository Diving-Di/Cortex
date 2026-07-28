CREATE TABLE public.research_jobs (
    id bigserial PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES public.tenants(id) ON DELETE CASCADE,
    created_by integer NOT NULL REFERENCES public.users(id),
    mode text NOT NULL CHECK (mode IN ('keyword','urls')),
    query_payload jsonb NOT NULL,
    target_count integer NOT NULL CHECK (target_count BETWEEN 1 AND 100),
    target_collection_id bigint,
    idempotency_key text NOT NULL,
    status text NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued','collecting','extracting','organizing','reviewing','completed','failed','cancelled')),
    found_count integer NOT NULL DEFAULT 0,
    collected_count integer NOT NULL DEFAULT 0,
    organized_count integer NOT NULL DEFAULT 0,
    failed_count integer NOT NULL DEFAULT 0,
    saved_count integer NOT NULL DEFAULT 0,
    attempt_count integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 3,
    available_at timestamptz NOT NULL DEFAULT now(),
    lease_owner text,
    lease_until timestamptz,
    last_error_code text,
    last_error_summary text,
    cancel_requested_at timestamptz,
    started_at timestamptz,
    completed_at timestamptz,
    version integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, idempotency_key),
    FOREIGN KEY (tenant_id, target_collection_id)
        REFERENCES public.knowledge_collections(tenant_id, id)
);

CREATE TABLE public.research_sources (
    id bigserial PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES public.tenants(id) ON DELETE CASCADE,
    job_id bigint NOT NULL,
    platform text NOT NULL DEFAULT 'xiaohongshu' CHECK (platform='xiaohongshu'),
    platform_source_id text,
    source_url text NOT NULL,
    normalized_url text NOT NULL,
    title text NOT NULL DEFAULT '',
    author_display_name text NOT NULL DEFAULT '',
    published_at timestamptz,
    raw_content text NOT NULL DEFAULT '',
    public_tags jsonb NOT NULL DEFAULT '[]'::jsonb,
    like_count bigint,
    collect_count bigint,
    comment_count bigint,
    content_hash char(64),
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','collecting','organizing','pending_review','saved','ignored','failed')),
    failure_code text,
    failure_summary text,
    collected_at timestamptz,
    version integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, normalized_url),
    FOREIGN KEY (tenant_id, job_id) REFERENCES public.research_jobs(tenant_id, id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX uq_research_sources_platform_id
    ON public.research_sources(tenant_id, platform, platform_source_id)
    WHERE platform_source_id IS NOT NULL AND deleted_at IS NULL;

CREATE TABLE public.research_assets (
    id bigserial PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES public.tenants(id) ON DELETE CASCADE,
    source_id bigint NOT NULL,
    position integer NOT NULL CHECK (position >= 0),
    storage_path text NOT NULL,
    original_url_hash char(64) NOT NULL,
    mime_type text NOT NULL,
    byte_size bigint NOT NULL CHECK (byte_size > 0),
    sha256 char(64) NOT NULL,
    width integer,
    height integer,
    ocr_status text NOT NULL DEFAULT 'pending'
        CHECK (ocr_status IN ('pending','processing','ready','failed','unavailable')),
    ocr_text text NOT NULL DEFAULT '',
    failure_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, source_id, position),
    FOREIGN KEY (tenant_id, source_id) REFERENCES public.research_sources(tenant_id, id) ON DELETE CASCADE
);

CREATE TABLE public.research_drafts (
    id bigserial PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES public.tenants(id) ON DELETE CASCADE,
    source_id bigint NOT NULL,
    summary text NOT NULL DEFAULT '',
    key_points jsonb NOT NULL DEFAULT '[]'::jsonb,
    category text NOT NULL DEFAULT '',
    suggested_tags jsonb NOT NULL DEFAULT '[]'::jsonb,
    edited_by_user boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','saved','ignored')),
    knowledge_document_id bigint,
    model_name text,
    prompt_version text NOT NULL DEFAULT 'research-v1',
    source_snapshot_hash char(64) NOT NULL,
    version integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, source_id),
    FOREIGN KEY (tenant_id, source_id) REFERENCES public.research_sources(tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, knowledge_document_id)
        REFERENCES public.knowledge_documents(tenant_id, id)
);

CREATE TABLE public.research_draft_revisions (
    id bigserial PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES public.tenants(id) ON DELETE CASCADE,
    draft_id bigint NOT NULL,
    summary text NOT NULL,
    key_points jsonb NOT NULL,
    category text NOT NULL,
    suggested_tags jsonb NOT NULL,
    reason text NOT NULL,
    created_by integer REFERENCES public.users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id, draft_id) REFERENCES public.research_drafts(tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX ix_research_jobs_claim
    ON public.research_jobs(status, available_at, lease_until, created_at)
    WHERE status IN ('queued','collecting','extracting','organizing');
CREATE INDEX ix_research_jobs_tenant_created
    ON public.research_jobs(tenant_id, created_at DESC);
CREATE INDEX ix_research_sources_job_status
    ON public.research_sources(tenant_id, job_id, status, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX ix_research_sources_search
    ON public.research_sources(tenant_id, lower(title), lower(author_display_name))
    WHERE deleted_at IS NULL;
CREATE INDEX ix_research_assets_source
    ON public.research_assets(tenant_id, source_id, position);

ALTER TABLE public.research_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.research_jobs FORCE ROW LEVEL SECURITY;
ALTER TABLE public.research_sources ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.research_sources FORCE ROW LEVEL SECURITY;
ALTER TABLE public.research_assets ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.research_assets FORCE ROW LEVEL SECURITY;
ALTER TABLE public.research_drafts ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.research_drafts FORCE ROW LEVEL SECURITY;
ALTER TABLE public.research_draft_revisions ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.research_draft_revisions FORCE ROW LEVEL SECURITY;

CREATE POLICY research_jobs_tenant_isolation ON public.research_jobs
    USING (tenant_id=NULLIF(current_setting('app.current_tenant_id',true),'')::uuid)
    WITH CHECK (tenant_id=NULLIF(current_setting('app.current_tenant_id',true),'')::uuid);
CREATE POLICY research_sources_tenant_isolation ON public.research_sources
    USING (tenant_id=NULLIF(current_setting('app.current_tenant_id',true),'')::uuid)
    WITH CHECK (tenant_id=NULLIF(current_setting('app.current_tenant_id',true),'')::uuid);
CREATE POLICY research_assets_tenant_isolation ON public.research_assets
    USING (tenant_id=NULLIF(current_setting('app.current_tenant_id',true),'')::uuid)
    WITH CHECK (tenant_id=NULLIF(current_setting('app.current_tenant_id',true),'')::uuid);
CREATE POLICY research_drafts_tenant_isolation ON public.research_drafts
    USING (tenant_id=NULLIF(current_setting('app.current_tenant_id',true),'')::uuid)
    WITH CHECK (tenant_id=NULLIF(current_setting('app.current_tenant_id',true),'')::uuid);
CREATE POLICY research_draft_revisions_tenant_isolation ON public.research_draft_revisions
    USING (tenant_id=NULLIF(current_setting('app.current_tenant_id',true),'')::uuid)
    WITH CHECK (tenant_id=NULLIF(current_setting('app.current_tenant_id',true),'')::uuid);

GRANT SELECT,INSERT,UPDATE,DELETE ON public.research_jobs TO diary_app;
GRANT SELECT,INSERT,UPDATE,DELETE ON public.research_sources TO diary_app;
GRANT SELECT,INSERT,UPDATE,DELETE ON public.research_assets TO diary_app;
GRANT SELECT,INSERT,UPDATE,DELETE ON public.research_drafts TO diary_app;
GRANT SELECT,INSERT,UPDATE,DELETE ON public.research_draft_revisions TO diary_app;
GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO diary_app;
