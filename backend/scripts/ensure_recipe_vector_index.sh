#!/usr/bin/env bash
set -euo pipefail

: ${DATABASE_URL:?Need DATABASE_URL}

echo "Ensuring pgvector extension and recipe child chunk indexes..."

psql "$DATABASE_URL" <<'SQL'
CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;
-- create tsvector index if not exists
CREATE INDEX IF NOT EXISTS ix_recipe_child_chunks_search ON public.recipe_child_chunks USING gin (search_vector);
-- create ivfflat index for embedding; note: requires REINDEX or VACUUM ANALYZE after bulk load for performance tuning
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relname='ix_recipe_child_chunks_embedding') THEN
    EXECUTE 'CREATE INDEX ix_recipe_child_chunks_embedding ON public.recipe_child_chunks USING ivfflat (embedding vector_l2_ops) WITH (lists = 100)';
  END IF;
END$$;
SQL

echo "Index creation attempted. For large datasets consider building the ivfflat index with CONCURRENTLY and tuning 'lists'." 
