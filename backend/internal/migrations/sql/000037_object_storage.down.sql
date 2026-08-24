DROP TABLE IF EXISTS public.object_gc_jobs;
DROP INDEX IF EXISTS public.ux_attachments_object_key;
ALTER TABLE public.knowledge_assets DROP COLUMN IF EXISTS etag,DROP COLUMN IF EXISTS object_version,DROP COLUMN IF EXISTS object_key,DROP COLUMN IF EXISTS storage_backend;
ALTER TABLE public.knowledge_documents DROP COLUMN IF EXISTS etag,DROP COLUMN IF EXISTS object_version,DROP COLUMN IF EXISTS object_key,DROP COLUMN IF EXISTS storage_backend;
ALTER TABLE public.attachments DROP COLUMN IF EXISTS deleted_at,DROP COLUMN IF EXISTS etag,DROP COLUMN IF EXISTS object_version,DROP COLUMN IF EXISTS object_key,DROP COLUMN IF EXISTS storage_backend;
