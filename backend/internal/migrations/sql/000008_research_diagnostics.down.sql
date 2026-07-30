ALTER TABLE public.research_sources
    DROP COLUMN IF EXISTS format_status,
    DROP COLUMN IF EXISTS formatted_content,
    DROP COLUMN IF EXISTS ocr_contribution_chars,
    DROP COLUMN IF EXISTS content_completeness,
    DROP COLUMN IF EXISTS parse_strategy;
