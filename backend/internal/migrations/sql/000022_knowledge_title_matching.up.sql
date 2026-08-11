-- pg_trgm is part of the baseline schema. Keep an explicit migration for
-- installations that reached the knowledge migrations from an older baseline.
CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;
