DROP INDEX IF EXISTS ix_knowledge_child_fts;
ALTER TABLE knowledge_child_chunks DROP COLUMN search_vector;
ALTER TABLE knowledge_child_chunks DROP COLUMN keyword_text;
ALTER TABLE knowledge_child_chunks
    ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (to_tsvector('simple', embedding_text)) STORED;
CREATE INDEX ix_knowledge_child_fts ON knowledge_child_chunks USING gin(search_vector);
