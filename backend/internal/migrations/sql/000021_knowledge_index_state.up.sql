ALTER TABLE knowledge_documents
    ADD COLUMN last_index_failure_code varchar(80);

-- Documents with an active version remain serviceable even if an older code
-- path left them in indexing/failed while a rebuild job was running.
UPDATE knowledge_documents
SET status = 'ready'
WHERE active_index_version > 0
  AND status IN ('indexing', 'failed')
  AND deleted_at IS NULL
  AND knowledge_enabled;
