ALTER TABLE public.research_drafts
    DROP COLUMN IF EXISTS knowledge_document_id;

ALTER TABLE public.research_jobs
    DROP COLUMN IF EXISTS target_collection_id;

DROP TABLE IF EXISTS public.knowledge_message_sources CASCADE;
DROP TABLE IF EXISTS public.knowledge_index_jobs CASCADE;
DROP TABLE IF EXISTS public.knowledge_child_chunks CASCADE;
DROP TABLE IF EXISTS public.knowledge_parent_chunks CASCADE;
DROP TABLE IF EXISTS public.knowledge_documents CASCADE;
DROP TABLE IF EXISTS public.knowledge_collections CASCADE;

ALTER TABLE public.tenants
    DROP COLUMN IF EXISTS knowledge_quota_bytes;
