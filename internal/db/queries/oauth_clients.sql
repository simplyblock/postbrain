-- name: RegisterClient :one
INSERT INTO oauth_clients (
    client_id,
    client_secret_hash,
    name,
    redirect_uris,
    grant_types,
    scopes,
    is_public,
    meta
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, client_id, client_secret_hash, name, redirect_uris, grant_types, scopes, is_public, meta, created_at, revoked_at;

-- name: LookupClient :one
SELECT id, client_id, client_secret_hash, name, redirect_uris, grant_types, scopes, is_public, meta, created_at, revoked_at
FROM oauth_clients
WHERE client_id = $1
  AND revoked_at IS NULL;

-- name: RevokeClient :exec
UPDATE oauth_clients
SET revoked_at = now()
WHERE id = $1;
