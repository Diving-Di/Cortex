ALTER TABLE public.knowledge_index_jobs
    DROP CONSTRAINT IF EXISTS knowledge_index_jobs_progress_bounds,
    DROP COLUMN IF EXISTS total_chunks,
    DROP COLUMN IF EXISTS processed_chunks,
    DROP COLUMN IF EXISTS stage;
