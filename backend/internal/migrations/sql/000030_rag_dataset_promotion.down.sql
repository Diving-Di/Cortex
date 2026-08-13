DROP TABLE IF EXISTS knowledge_eval_cases;
DROP TABLE IF EXISTS knowledge_eval_datasets;
ALTER TABLE knowledge_rag_feedback DROP CONSTRAINT IF EXISTS knowledge_rag_feedback_tenant_trace_fkey;
ALTER TABLE knowledge_rag_feedback ADD CONSTRAINT knowledge_rag_feedback_trace_id_fkey FOREIGN KEY (trace_id) REFERENCES knowledge_rag_traces(id) ON DELETE CASCADE;
ALTER TABLE knowledge_rag_feedback DROP CONSTRAINT IF EXISTS knowledge_rag_feedback_tenant_id_id_key;
ALTER TABLE knowledge_rag_traces DROP CONSTRAINT IF EXISTS knowledge_rag_traces_tenant_id_id_key;
ALTER TABLE knowledge_rag_feedback DROP COLUMN IF EXISTS reviewed_at, DROP COLUMN IF EXISTS review_summary;
