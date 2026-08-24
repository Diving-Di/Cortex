ALTER TABLE public.attachments ADD CONSTRAINT attachments_storage_locator_check CHECK((storage_backend='local' AND stored_path<>'') OR (storage_backend='minio' AND object_key IS NOT NULL));
CREATE INDEX ix_outbox_kafka_publish ON public.outbox_events(publish_status,available_at,lease_until) WHERE processed_at IS NULL;
