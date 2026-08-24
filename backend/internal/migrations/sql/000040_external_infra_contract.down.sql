DROP INDEX IF EXISTS public.ix_outbox_kafka_publish;ALTER TABLE public.attachments DROP CONSTRAINT IF EXISTS attachments_storage_locator_check;
