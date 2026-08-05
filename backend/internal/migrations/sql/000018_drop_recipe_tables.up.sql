-- Drop legacy recipe tables. The recipe feature and its interfaces were removed;
-- these tables are no longer read or written by any code.

DROP TABLE IF EXISTS public.recipe_message_sources;
DROP TABLE IF EXISTS public.recipe_child_chunks;
DROP TABLE IF EXISTS public.recipe_parent_chunks;
DROP TABLE IF EXISTS public.recipe_index_jobs;
DROP TABLE IF EXISTS public.recipe_sync_runs;
DROP TABLE IF EXISTS public.recipe_documents;
