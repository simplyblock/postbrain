-- name: CreateScopeGrant :one
INSERT INTO scope_grants (principal_id, scope_id, permissions, granted_by, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, principal_id, scope_id, permissions, granted_by, expires_at, created_at;

-- name: GetScopeGrant :one
SELECT id, principal_id, scope_id, permissions, granted_by, expires_at, created_at
FROM scope_grants
WHERE principal_id = $1 AND scope_id = $2;

-- name: ListScopeGrantsByPrincipal :many
SELECT id, principal_id, scope_id, permissions, granted_by, expires_at, created_at
FROM scope_grants
WHERE principal_id = $1
  AND (expires_at IS NULL OR expires_at > now())
ORDER BY created_at DESC;

-- name: ListScopeGrantsByScope :many
SELECT id, principal_id, scope_id, permissions, granted_by, expires_at, created_at
FROM scope_grants
WHERE scope_id = $1
  AND (expires_at IS NULL OR expires_at > now())
ORDER BY created_at DESC;

-- name: UpdateScopeGrantPermissions :one
UPDATE scope_grants
SET permissions = $3
WHERE principal_id = $1 AND scope_id = $2
RETURNING id, principal_id, scope_id, permissions, granted_by, expires_at, created_at;

-- name: DeleteScopeGrant :exec
DELETE FROM scope_grants WHERE id = $1;

-- name: DeleteExpiredScopeGrants :exec
DELETE FROM scope_grants WHERE expires_at IS NOT NULL AND expires_at <= now();
