DROP INDEX IF EXISTS public.ix_template_reports_review;
ALTER TABLE public.template_reports
    DROP CONSTRAINT IF EXISTS ck_template_report_status,
    DROP COLUMN IF EXISTS reviewed_at,
    DROP COLUMN IF EXISTS review_note,
    DROP COLUMN IF EXISTS reviewer_id,
    DROP COLUMN IF EXISTS status;
DROP INDEX IF EXISTS public.ix_published_template_search_trgm;
