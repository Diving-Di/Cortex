CREATE TABLE knowledge_quotas (
    tenant_id uuid PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    used_bytes bigint NOT NULL DEFAULT 0 CHECK (used_bytes >= 0),
    reserved_bytes bigint NOT NULL DEFAULT 0 CHECK (reserved_bytes >= 0),
    reservation_expires_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (used_bytes + reserved_bytes <= 3221225472)
);

CREATE TABLE knowledge_collections (
    id uuid NOT NULL DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL,
    name varchar(120) NOT NULL, description text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    PRIMARY KEY (tenant_id, id), FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX uq_knowledge_collections_name ON knowledge_collections(tenant_id, lower(name)) WHERE deleted_at IS NULL;

CREATE TABLE knowledge_uploads (
    id uuid NOT NULL DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL,
    idempotency_key varchar(200), original_name text NOT NULL, stored_root text NOT NULL,
    reserved_bytes bigint NOT NULL DEFAULT 0, expanded_bytes bigint NOT NULL DEFAULT 0,
    status varchar(20) NOT NULL CHECK (status IN ('uploaded','parsing','indexing','ready','failed','deleting')),
    failure_code varchar(80), failure_summary varchar(500), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id), FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
    UNIQUE (tenant_id, idempotency_key), CHECK (stored_root !~ '(^[/\\]|(^|[/\\])\.\.([/\\]|$)|^[A-Za-z]:)')
);

CREATE TABLE knowledge_documents (
    id uuid NOT NULL DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, upload_id uuid, collection_id uuid,
    source_type varchar(10) NOT NULL CHECK (source_type IN ('upload','note')), note_id integer,
    title text NOT NULL, stored_path text, source_encoding varchar(20), size_bytes bigint NOT NULL DEFAULT 0,
    content_hash char(64) NOT NULL, active_index_version integer NOT NULL DEFAULT 0,
    status varchar(20) NOT NULL CHECK (status IN ('uploaded','parsing','indexing','ready','failed','deleting')),
    knowledge_enabled boolean NOT NULL DEFAULT true, failure_code varchar(80), failure_summary varchar(500),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    PRIMARY KEY (tenant_id, id), FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, upload_id) REFERENCES knowledge_uploads(tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, collection_id) REFERENCES knowledge_collections(tenant_id, id) ON DELETE SET NULL (collection_id),
    FOREIGN KEY (tenant_id, note_id) REFERENCES notes(tenant_id, id) ON DELETE CASCADE,
    CHECK ((source_type='upload' AND upload_id IS NOT NULL AND stored_path IS NOT NULL AND note_id IS NULL) OR
           (source_type='note' AND upload_id IS NULL AND stored_path IS NULL AND note_id IS NOT NULL)),
    CHECK (stored_path IS NULL OR stored_path !~ '(^[/\\]|(^|[/\\])\.\.([/\\]|$)|^[A-Za-z]:)')
);
CREATE UNIQUE INDEX uq_knowledge_note ON knowledge_documents(tenant_id, note_id) WHERE source_type='note';
CREATE INDEX ix_knowledge_documents_active ON knowledge_documents(tenant_id, status, collection_id) WHERE deleted_at IS NULL AND knowledge_enabled;

CREATE TABLE knowledge_assets (
    id uuid NOT NULL DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, document_id uuid NOT NULL,
    stored_path text NOT NULL, mime_type varchar(80) NOT NULL, size_bytes bigint NOT NULL, sha256 char(64) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, document_id) REFERENCES knowledge_documents(tenant_id, id) ON DELETE CASCADE,
    UNIQUE (tenant_id, document_id, stored_path), CHECK (stored_path !~ '(^[/\\]|(^|[/\\])\.\.([/\\]|$)|^[A-Za-z]:)')
);

CREATE TABLE knowledge_parent_chunks (
    id uuid NOT NULL DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, document_id uuid NOT NULL,
    index_version integer NOT NULL, ordinal integer NOT NULL, heading_path text[] NOT NULL DEFAULT '{}', content text NOT NULL, content_hash char(64) NOT NULL,
    PRIMARY KEY (tenant_id, id), FOREIGN KEY (tenant_id, document_id) REFERENCES knowledge_documents(tenant_id, id) ON DELETE CASCADE,
    UNIQUE (tenant_id, document_id, index_version, ordinal)
);
CREATE TABLE knowledge_child_chunks (
    id uuid NOT NULL DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, parent_id uuid NOT NULL,
    document_id uuid NOT NULL, index_version integer NOT NULL, ordinal integer NOT NULL,
    content text NOT NULL, embedding_text text NOT NULL, search_vector tsvector GENERATED ALWAYS AS (to_tsvector('simple', embedding_text)) STORED,
    embedding vector(512), embedding_model text, content_hash char(64) NOT NULL,
    PRIMARY KEY (tenant_id, id), FOREIGN KEY (tenant_id, parent_id) REFERENCES knowledge_parent_chunks(tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, document_id) REFERENCES knowledge_documents(tenant_id, id) ON DELETE CASCADE,
    UNIQUE (tenant_id, parent_id, ordinal)
);
CREATE INDEX ix_knowledge_child_fts ON knowledge_child_chunks USING gin(search_vector);
CREATE INDEX ix_knowledge_child_vector ON knowledge_child_chunks USING hnsw (embedding vector_cosine_ops) WHERE embedding IS NOT NULL;

CREATE TABLE knowledge_index_jobs (
    id bigserial PRIMARY KEY, tenant_id uuid NOT NULL, document_id uuid NOT NULL, target_index_version integer NOT NULL,
    status varchar(16) NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','success','failed')),
    lease_owner uuid, lease_until timestamptz, attempts integer NOT NULL DEFAULT 0, available_at timestamptz NOT NULL DEFAULT now(),
    failure_code varchar(80), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id, document_id) REFERENCES knowledge_documents(tenant_id, id) ON DELETE CASCADE,
    UNIQUE (tenant_id, document_id, target_index_version)
);
CREATE INDEX ix_knowledge_jobs_claim ON knowledge_index_jobs(status, available_at, lease_until);

CREATE TABLE knowledge_message_sources (
    id bigserial PRIMARY KEY, tenant_id uuid NOT NULL, message_id integer NOT NULL, source_type varchar(10) NOT NULL,
    document_id uuid, note_id integer, title text NOT NULL, snippet varchar(1000) NOT NULL, index_version integer NOT NULL,
    rank integer NOT NULL, source_deleted boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, message_id) REFERENCES messages(tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, document_id) REFERENCES knowledge_documents(tenant_id, id) ON DELETE SET NULL (document_id),
    UNIQUE (tenant_id, message_id, rank)
);

DO $$ DECLARE t text; BEGIN
  FOREACH t IN ARRAY ARRAY['knowledge_quotas','knowledge_collections','knowledge_uploads','knowledge_documents','knowledge_assets','knowledge_parent_chunks','knowledge_child_chunks','knowledge_index_jobs','knowledge_message_sources'] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format('CREATE POLICY %I ON %I USING (tenant_id = nullif(current_setting(''app.current_tenant_id'', true), '''')::uuid) WITH CHECK (tenant_id = nullif(current_setting(''app.current_tenant_id'', true), '''')::uuid)', t || '_tenant_isolation', t);
  END LOOP;
END $$;

GRANT SELECT, INSERT, UPDATE, DELETE ON
    knowledge_quotas, knowledge_collections, knowledge_uploads,
    knowledge_documents, knowledge_assets, knowledge_parent_chunks,
    knowledge_child_chunks, knowledge_index_jobs, knowledge_message_sources
TO diary_app;
GRANT USAGE, SELECT ON SEQUENCE knowledge_index_jobs_id_seq, knowledge_message_sources_id_seq TO diary_app;
