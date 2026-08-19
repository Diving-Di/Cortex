CREATE TABLE public.knowledge_clarifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    user_id integer NOT NULL,
    conversation_id integer NOT NULL,
    original_request_id varchar(128) NOT NULL,
    original_question text NOT NULL CHECK (char_length(original_question) BETWEEN 1 AND 5000),
    collection_ids uuid[] NOT NULL DEFAULT '{}',
    kind varchar(24) NOT NULL CHECK (kind IN ('ambiguous','scope_conflict')),
    prompt varchar(500) NOT NULL,
    status varchar(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','resumed','expired')),
    expires_at timestamptz NOT NULL,
    resumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id, user_id) REFERENCES public.tenants(id, user_id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, conversation_id) REFERENCES public.conversations(tenant_id, id) ON DELETE CASCADE,
    UNIQUE (tenant_id, user_id, original_request_id)
);

CREATE INDEX knowledge_clarifications_pending
    ON public.knowledge_clarifications(tenant_id, user_id, expires_at)
    WHERE status='pending';

ALTER TABLE public.knowledge_clarifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.knowledge_clarifications FORCE ROW LEVEL SECURITY;
CREATE POLICY knowledge_clarifications_tenant_isolation ON public.knowledge_clarifications
    USING (tenant_id = nullif(current_setting('app.current_tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.current_tenant_id', true), '')::uuid);
GRANT SELECT,INSERT,UPDATE,DELETE ON public.knowledge_clarifications TO cortex_app;
