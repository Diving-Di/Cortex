-- name: FindUserForLogin :one
SELECT id, username, password_hash
FROM users
WHERE username = $1;

-- name: InsertAuthToken :exec
INSERT INTO auth_tokens (token_hash, user_id, expires_at)
VALUES ($1, $2, $3);

-- name: RevokeAuthToken :exec
UPDATE auth_tokens SET revoked_at = now() WHERE id = $1;
