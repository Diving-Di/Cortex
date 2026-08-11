ALTER TABLE public.messages
    ADD COLUMN error_code varchar(64),
    ADD COLUMN upstream_stage varchar(64),
    ADD COLUMN output_tokens integer NOT NULL DEFAULT 0 CHECK (output_tokens >= 0);
