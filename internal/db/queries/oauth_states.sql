-- name: IssueState :one
INSERT INTO oauth_states (
    state_hash,
    kind,
    payload,
    expires_at
)
VALUES ($1, $2, $3, $4)
RETURNING id, state_hash, kind, payload, expires_at, used_at, created_at;

-- name: ConsumeState :one
UPDATE oauth_states
SET used_at = now()
WHERE state_hash = $1
  AND used_at IS NULL
  AND expires_at > now()
RETURNING id, state_hash, kind, payload, expires_at, used_at, created_at;

-- name: GetStateByHash :one
SELECT id, state_hash, kind, payload, expires_at, used_at, created_at
FROM oauth_states
WHERE state_hash = $1
  AND used_at IS NULL
  AND expires_at > now();
