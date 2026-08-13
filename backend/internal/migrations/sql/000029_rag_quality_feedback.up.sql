CREATE TABLE knowledge_rag_traces (
    id bigserial PRIMARY KEY,
    tenant_id uuid NOT NULL,
    user_id integer NOT NULL,
    request_id varchar(128) NOT NULL,
    message_id integer NOT NULL,
    status varchar(20) NOT NULL,
    error_code varchar(64),
    upstream_stage varchar(64),
    output_tokens integer NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    config_snapshot jsonb NOT NULL,
    source_snapshot jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, message_id) REFERENCES messages(tenant_id, id) ON DELETE CASCADE,
    UNIQUE (tenant_id, request_id)
);

CREATE TABLE knowledge_rag_feedback (
    id bigserial PRIMARY KEY,
    tenant_id uuid NOT NULL,
    user_id integer NOT NULL,
    trace_id bigint NOT NULL,
    category varchar(40) NOT NULL CHECK (category IN (
        'incorrect_answer','unsupported_citation','missing_knowledge',
        'should_have_refused','high_latency'
    )),
    comment varchar(1000) NOT NULL DEFAULT '',
    status varchar(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','reviewed','promoted','dismissed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
    FOREIGN KEY (trace_id) REFERENCES knowledge_rag_traces(id) ON DELETE CASCADE,
    UNIQUE (tenant_id, user_id, trace_id)
);

ALTER TABLE knowledge_rag_traces ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_rag_traces FORCE ROW LEVEL SECURITY;
CREATE POLICY knowledge_rag_traces_tenant_isolation ON knowledge_rag_traces
    USING (tenant_id = nullif(current_setting('app.current_tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.current_tenant_id', true), '')::uuid);
ALTER TABLE knowledge_rag_feedback ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_rag_feedback FORCE ROW LEVEL SECURITY;
CREATE POLICY knowledge_rag_feedback_tenant_isolation ON knowledge_rag_feedback
    USING (tenant_id = nullif(current_setting('app.current_tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.current_tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON knowledge_rag_traces, knowledge_rag_feedback TO diary_app;
GRANT USAGE, SELECT ON SEQUENCE knowledge_rag_traces_id_seq, knowledge_rag_feedback_id_seq TO diary_app;
