-- pg_trgm is shared by the notes title/content indexes, so this migration does
-- not remove the extension on rollback.
SELECT 1;
