-- Rollback recipe corpus migration

ALTER TABLE public.conversations DROP CONSTRAINT IF EXISTS conversations_source_scope_check;
ALTER TABLE public.conversations
    ADD CONSTRAINT conversations_source_scope_check CHECK (((source_scope)::text = ANY ((ARRAY['knowledge'::character varying, 'growth'::character varying, 'all'::character varying])::text[])));

REVOKE ALL ON public.user_preferences FROM diary_app;
REVOKE SELECT ON public.recipe_sync_runs FROM diary_app;
REVOKE SELECT ON public.recipe_child_chunks FROM diary_app;
REVOKE SELECT ON public.recipe_parent_chunks FROM diary_app;
REVOKE SELECT ON public.recipe_documents FROM diary_app;

REVOKE SELECT, INSERT ON public.recipe_message_sources FROM diary_app;

DROP TABLE IF EXISTS public.user_preferences;
DROP TABLE IF EXISTS public.recipe_sync_runs;
DROP TABLE IF EXISTS public.recipe_child_chunks;
DROP TABLE IF EXISTS public.recipe_parent_chunks;
DROP TABLE IF EXISTS public.recipe_documents;
DROP INDEX IF EXISTS ix_recipe_child_chunks_embedding;
DROP INDEX IF EXISTS ix_recipe_child_chunks_search;
DROP TABLE IF EXISTS public.recipe_message_sources;
