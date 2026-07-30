ALTER TABLE public.recipe_index_jobs DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS recipe_index_jobs_tenant_isolation ON public.recipe_index_jobs;
DROP TABLE IF EXISTS public.recipe_index_jobs;

-- revoke grants not strictly necessary in down migration but mirror up
REVOKE SELECT, INSERT, UPDATE, DELETE ON public.recipe_index_jobs FROM diary_app;
