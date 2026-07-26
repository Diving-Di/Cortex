-- These queries document the sqlc target. During the Alembic-owned PoC the
-- runtime store uses equivalent explicit pgx queries so no generated source is
-- required to build. They become authoritative at the migration hand-off.

-- name: GetNote :one
SELECT id, type, title, content, note_date, summary, word_count, created_at, updated_at
FROM notes
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: CountActiveNotes :one
SELECT count(*)
FROM notes
WHERE tenant_id = $1 AND deleted_at IS NULL;

-- name: ListNoteRevisions :many
SELECT id, note_id, content, reason, created_at
FROM note_revisions
WHERE tenant_id = $1 AND note_id = $2
ORDER BY created_at DESC;
