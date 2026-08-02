ALTER TABLE public.recipe_documents
    ADD COLUMN active_index_version integer NOT NULL DEFAULT 1,
    ADD CONSTRAINT recipe_documents_active_index_version_check CHECK (active_index_version > 0);

ALTER TABLE public.recipe_parent_chunks
    ADD COLUMN index_version integer NOT NULL DEFAULT 1,
    ADD CONSTRAINT recipe_parent_chunks_index_version_check CHECK (index_version > 0);

DROP INDEX IF EXISTS recipe_parent_chunks_document_idx;
CREATE INDEX recipe_parent_chunks_document_idx
    ON public.recipe_parent_chunks (document_id, index_version, parent_index);

DROP INDEX IF EXISTS ix_recipe_child_chunks_embedding;
CREATE INDEX ix_recipe_child_chunks_embedding
    ON public.recipe_child_chunks
    USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
