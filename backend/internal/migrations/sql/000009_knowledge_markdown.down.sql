ALTER TABLE public.knowledge_documents
    DROP CONSTRAINT knowledge_documents_extension_check;

ALTER TABLE public.knowledge_documents
    ADD CONSTRAINT knowledge_documents_extension_check
    CHECK (extension IN ('.txt', '.pdf', '.docx')) NOT VALID;
