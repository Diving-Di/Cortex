DROP INDEX IF EXISTS public.ix_object_gc_jobs_claim;

ALTER TABLE public.object_gc_jobs
  DROP COLUMN IF EXISTS lease_expires_at,
  DROP COLUMN IF EXISTS lease_owner;
