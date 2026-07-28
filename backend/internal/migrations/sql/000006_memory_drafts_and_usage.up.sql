ALTER TABLE public.ai_usage_records ADD COLUMN conversation_id integer;
ALTER TABLE public.ai_usage_records ADD CONSTRAINT fk_ai_usage_conversation
  FOREIGN KEY (tenant_id,conversation_id) REFERENCES public.conversations(tenant_id,id) ON DELETE SET NULL (conversation_id);
