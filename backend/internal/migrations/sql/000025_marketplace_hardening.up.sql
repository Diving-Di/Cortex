CREATE INDEX IF NOT EXISTS ix_published_template_search_trgm
    ON public.published_template_snapshots
    USING gin ((coalesce(title,'') || ' ' || coalesce(description,'')) gin_trgm_ops)
    WHERE status='published';

ALTER TABLE public.template_reports
    ADD COLUMN status varchar(20) NOT NULL DEFAULT 'pending',
    ADD COLUMN reviewer_id integer REFERENCES public.users(id),
    ADD COLUMN review_note varchar(1000) NOT NULL DEFAULT '',
    ADD COLUMN reviewed_at timestamptz,
    ADD CONSTRAINT ck_template_report_status
        CHECK (status IN ('pending','reviewing','resolved','rejected'));

CREATE INDEX ix_template_reports_review
    ON public.template_reports(status, created_at, id);
