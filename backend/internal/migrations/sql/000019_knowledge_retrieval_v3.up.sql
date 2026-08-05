-- Rebuild every enabled ready document with the v3 chunker. The active v2
-- index remains queryable until the worker atomically activates this version.
INSERT INTO knowledge_index_jobs (tenant_id, document_id, target_index_version, status, available_at)
SELECT tenant_id, id, active_index_version + 1, 'queued', now()
FROM knowledge_documents
WHERE status = 'ready'
  AND deleted_at IS NULL
  AND knowledge_enabled
  AND active_index_version > 0
ON CONFLICT (tenant_id, document_id, target_index_version)
DO UPDATE SET status = 'queued', available_at = now(), failure_code = NULL,
              lease_owner = NULL, lease_until = NULL, updated_at = now();
