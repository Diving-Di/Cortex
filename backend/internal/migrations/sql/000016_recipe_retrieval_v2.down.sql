DROP INDEX IF EXISTS public.ix_recipe_child_chunks_embedding;
CREATE INDEX ix_recipe_child_chunks_embedding
    ON public.recipe_child_chunks
    USING ivfflat (embedding vector_l2_ops) WITH (lists = 100);

DROP INDEX IF EXISTS public.recipe_parent_chunks_document_idx;
CREATE INDEX recipe_parent_chunks_document_idx
    ON public.recipe_parent_chunks (document_id);

ALTER TABLE public.recipe_parent_chunks
    DROP CONSTRAINT IF EXISTS recipe_parent_chunks_index_version_check,
    DROP COLUMN IF EXISTS index_version;

ALTER TABLE public.recipe_documents
    DROP CONSTRAINT IF EXISTS recipe_documents_active_index_version_check,
    DROP COLUMN IF EXISTS active_index_version;
