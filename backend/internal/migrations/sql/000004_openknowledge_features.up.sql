ALTER TABLE public.conversations
  ADD COLUMN version integer NOT NULL DEFAULT 1,
  ADD COLUMN summary text,
  ADD COLUMN summary_through_message_id integer,
  ADD COLUMN summary_version integer NOT NULL DEFAULT 0,
  ADD COLUMN summary_model varchar(120),
  ADD COLUMN summary_updated_at timestamptz;
