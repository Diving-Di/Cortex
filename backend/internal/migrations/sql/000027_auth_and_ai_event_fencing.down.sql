DROP TABLE IF EXISTS public.ai_event_reservations;
DROP INDEX IF EXISTS public.ai_flash_claims_reservation_token_unique;
ALTER TABLE public.ai_flash_claims DROP COLUMN IF EXISTS reservation_version, DROP COLUMN IF EXISTS reservation_token;
ALTER TABLE public.tenants DROP COLUMN IF EXISTS auth_version;
ALTER TABLE public.auth_tokens DROP COLUMN IF EXISTS auth_version;
