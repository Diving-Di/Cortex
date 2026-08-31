ALTER TABLE public.object_gc_jobs
  ADD COLUMN lease_owner uuid,
  ADD COLUMN lease_expires_at timestamptz;

UPDATE public.object_gc_jobs
SET status = 'queued', available_at = now(), updated_at = now()
WHERE status = 'running';

CREATE INDEX ix_object_gc_jobs_claim
  ON public.object_gc_jobs(status, available_at, lease_expires_at, id);
