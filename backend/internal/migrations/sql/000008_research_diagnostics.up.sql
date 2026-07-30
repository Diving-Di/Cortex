ALTER TABLE public.research_sources
    ADD COLUMN parse_strategy text NOT NULL DEFAULT 'metadata',
    ADD COLUMN content_completeness smallint NOT NULL DEFAULT 0
        CHECK (content_completeness BETWEEN 0 AND 100),
    ADD COLUMN ocr_contribution_chars integer NOT NULL DEFAULT 0
        CHECK (ocr_contribution_chars >= 0),
    ADD COLUMN formatted_content text NOT NULL DEFAULT '',
    ADD COLUMN format_status text NOT NULL DEFAULT 'deterministic'
        CHECK (format_status IN ('deterministic','ai_formatted','ai_unavailable','ai_failed'));
