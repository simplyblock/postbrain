-- name: IssueCode :one
INSERT INTO oauth_auth_codes (
    code_hash,
    client_id,
    principal_id,
    redirect_uri,
    scopes,
    code_challenge,
    expires_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, code_hash, client_id, principal_id, redirect_uri, scopes, code_challenge, expires_at, used_at, created_at;

-- name: ConsumeCode :one
UPDATE oauth_auth_codes
SET used_at = now()
WHERE code_hash = $1
  AND used_at IS NULL
  AND expires_at > now()
RETURNING id, code_hash, client_id, principal_id, redirect_uri, scopes, code_challenge, expires_at, used_at, created_at;

-- name: GetCodeByHash :one
SELECT id, code_hash, client_id, principal_id, redirect_uri, scopes, code_challenge, expires_at, used_at, created_at
FROM oauth_auth_codes
WHERE code_hash = $1;
