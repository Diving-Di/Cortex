ALTER TABLE knowledge_rag_feedback
    ADD COLUMN review_summary varchar(1000),
    ADD COLUMN reviewed_at timestamptz;

ALTER TABLE knowledge_rag_traces ADD CONSTRAINT knowledge_rag_traces_tenant_id_id_key UNIQUE (tenant_id, id);
ALTER TABLE knowledge_rag_feedback ADD CONSTRAINT knowledge_rag_feedback_tenant_id_id_key UNIQUE (tenant_id, id);
ALTER TABLE knowledge_rag_feedback DROP CONSTRAINT knowledge_rag_feedback_trace_id_fkey;
ALTER TABLE knowledge_rag_feedback ADD CONSTRAINT knowledge_rag_feedback_tenant_trace_fkey
    FOREIGN KEY (tenant_id, trace_id) REFERENCES knowledge_rag_traces(tenant_id, id) ON DELETE CASCADE;

CREATE TABLE knowledge_eval_datasets (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    user_id integer NOT NULL,
    name varchar(120) NOT NULL,
    version integer NOT NULL CHECK (version > 0),
    status varchar(20) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','frozen')),
    manifest_sha256 char(64),
    case_count integer NOT NULL DEFAULT 0 CHECK (case_count >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    frozen_at timestamptz,
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
    UNIQUE (tenant_id, user_id, name, version),
    CHECK ((status='draft' AND manifest_sha256 IS NULL AND frozen_at IS NULL) OR
           (status='frozen' AND manifest_sha256 IS NOT NULL AND frozen_at IS NOT NULL))
);

CREATE TABLE knowledge_eval_cases (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    dataset_id uuid NOT NULL,
    feedback_id bigint NOT NULL,
    case_id varchar(120) NOT NULL,
    query_text varchar(2000) NOT NULL,
    expected_answer varchar(4000) NOT NULL,
    evidence_hashes jsonb NOT NULL,
    tags jsonb NOT NULL DEFAULT '[]'::jsonb,
    case_sha256 char(64) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, dataset_id) REFERENCES knowledge_eval_datasets(tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, feedback_id) REFERENCES knowledge_rag_feedback(tenant_id, id) ON DELETE RESTRICT,
    UNIQUE (tenant_id, dataset_id, case_id),
    UNIQUE (tenant_id, dataset_id, feedback_id),
    CHECK (jsonb_typeof(evidence_hashes)='array' AND jsonb_array_length(evidence_hashes)>0),
    CHECK (jsonb_typeof(tags)='array')
);

ALTER TABLE knowledge_eval_datasets ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_eval_datasets FORCE ROW LEVEL SECURITY;
CREATE POLICY knowledge_eval_datasets_tenant_isolation ON knowledge_eval_datasets
    USING (tenant_id = nullif(current_setting('app.current_tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.current_tenant_id', true), '')::uuid);
ALTER TABLE knowledge_eval_cases ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_eval_cases FORCE ROW LEVEL SECURITY;
CREATE POLICY knowledge_eval_cases_tenant_isolation ON knowledge_eval_cases
    USING (tenant_id = nullif(current_setting('app.current_tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.current_tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON knowledge_eval_datasets, knowledge_eval_cases TO diary_app;
