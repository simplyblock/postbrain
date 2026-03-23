-- name: CreateToken :one
INSERT INTO tokens (principal_id, token_hash, name, scope_ids, permissions, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, principal_id, token_hash, name, scope_ids, permissions, expires_at, last_used_at, created_at, revoked_at;

-- name: LookupToken :one
SELECT id, principal_id, token_hash, name, scope_ids, permissions, expires_at, last_used_at, created_at, revoked_at
FROM tokens WHERE token_hash = $1;

-- name: RevokeToken :exec
UPDATE tokens SET revoked_at = now() WHERE id = $1;

-- name: UpdateTokenLastUsed :exec
UPDATE tokens SET last_used_at = now() WHERE id = $1;

-- name: ListAllTokens :many
SELECT id, principal_id, token_hash, name, scope_ids, permissions, expires_at, last_used_at, created_at, revoked_at
FROM tokens
ORDER BY created_at DESC;

-- name: ListTokensByPrincipal :many
SELECT id, principal_id, token_hash, name, scope_ids, permissions, expires_at, last_used_at, created_at, revoked_at
FROM tokens
WHERE principal_id = $1
ORDER BY created_at DESC;
