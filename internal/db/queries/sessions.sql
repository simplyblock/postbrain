-- name: CreateSession :one
INSERT INTO sessions (scope_id, principal_id, meta)
VALUES ($1, $2, $3)
RETURNING id, scope_id, principal_id, started_at, ended_at, meta;

-- name: GetSession :one
SELECT id, scope_id, principal_id, started_at, ended_at, meta
FROM sessions WHERE id = $1;

-- name: EndSession :one
UPDATE sessions
SET ended_at = COALESCE($2::timestamptz, now()),
    meta     = CASE WHEN $3::jsonb IS NOT NULL THEN $3 ELSE meta END
WHERE id = $1
RETURNING id, scope_id, principal_id, started_at, ended_at, meta;
