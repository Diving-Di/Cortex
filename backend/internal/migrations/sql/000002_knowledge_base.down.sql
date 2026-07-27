DROP INDEX IF EXISTS public.uq_messages_tenant_request;
ALTER TABLE public.messages DROP COLUMN IF EXISTS request_id;
ALTER TABLE public.messages DROP COLUMN IF EXISTS status;
ALTER TABLE public.conversations DROP COLUMN IF EXISTS source_scope;

DROP TABLE IF EXISTS public.knowledge_message_sources;
DROP TABLE IF EXISTS public.knowledge_index_jobs;
DROP TABLE IF EXISTS public.knowledge_child_chunks;
DROP TABLE IF EXISTS public.knowledge_parent_chunks;
DROP TABLE IF EXISTS public.knowledge_documents;
DROP TABLE IF EXISTS public.knowledge_collections;

ALTER TABLE public.tenants DROP COLUMN IF EXISTS knowledge_quota_bytes;

DROP EXTENSION IF EXISTS vector;
