DROP TABLE IF EXISTS public.knowledge_index_artifacts;

UPDATE public.knowledge_index_jobs SET stage='queued',status='queued' WHERE stage='parsed';

ALTER TABLE public.knowledge_index_jobs
    DROP CONSTRAINT knowledge_index_jobs_stage_check;

ALTER TABLE public.knowledge_index_jobs
    ADD CONSTRAINT knowledge_index_jobs_stage_check
        CHECK (stage IN ('queued','loading','parsing','embedding','persisting','completed','failed'));
