DROP TABLE IF EXISTS public.growth_memory_drafts;
DROP TABLE IF EXISTS public.memory_settings;
DROP TABLE IF EXISTS public.growth_memories;
ALTER TABLE public.conversations
  DROP COLUMN IF EXISTS summary_updated_at,
  DROP COLUMN IF EXISTS summary_model,
  DROP COLUMN IF EXISTS summary_version,
  DROP COLUMN IF EXISTS summary_through_message_id,
  DROP COLUMN IF EXISTS summary,
  DROP COLUMN IF EXISTS version;
