ALTER TABLE knowledge_child_chunks DROP COLUMN search_vector;
ALTER TABLE knowledge_child_chunks
    ADD COLUMN keyword_text text NOT NULL DEFAULT '',
    ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (to_tsvector('simple', keyword_text)) STORED;

CREATE INDEX ix_knowledge_child_fts ON knowledge_child_chunks USING gin(search_vector);

-- Keep the current active version queryable while workers build the next
-- version with application-generated Unicode 2-gram lexemes.
INSERT INTO knowledge_index_jobs(tenant_id, document_id, target_index_version, status, available_at)
SELECT d.tenant_id, d.id, d.active_index_version + 1, 'queued', now()
FROM knowledge_documents d
WHERE d.status = 'ready'
  AND d.deleted_at IS NULL
  AND d.knowledge_enabled
  AND d.active_index_version > 0
ON CONFLICT (tenant_id, document_id, target_index_version) DO NOTHING;
