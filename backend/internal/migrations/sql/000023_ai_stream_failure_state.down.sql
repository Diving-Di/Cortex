ALTER TABLE public.messages
    DROP COLUMN IF EXISTS output_tokens,
    DROP COLUMN IF EXISTS upstream_stage,
    DROP COLUMN IF EXISTS error_code;
