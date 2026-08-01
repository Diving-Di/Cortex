ALTER TABLE public.ai_flash_events
    ADD COLUMN reservation_ready boolean NOT NULL DEFAULT false;
