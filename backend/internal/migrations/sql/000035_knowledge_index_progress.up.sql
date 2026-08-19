ALTER TABLE public.knowledge_index_jobs
    ADD COLUMN stage varchar(20) NOT NULL DEFAULT 'queued'
        CHECK (stage IN ('queued','loading','parsing','embedding','persisting','completed','failed')),
    ADD COLUMN processed_chunks integer NOT NULL DEFAULT 0 CHECK (processed_chunks >= 0),
    ADD COLUMN total_chunks integer NOT NULL DEFAULT 0 CHECK (total_chunks >= 0),
    ADD CONSTRAINT knowledge_index_jobs_progress_bounds CHECK (processed_chunks <= total_chunks);

UPDATE public.knowledge_index_jobs
SET stage = CASE status
    WHEN 'success' THEN 'completed'
    WHEN 'failed' THEN 'failed'
    WHEN 'running' THEN 'loading'
    ELSE 'queued'
END;
