-- name: CreateScope :one
INSERT INTO scopes (kind, external_id, name, parent_id, principal_id, meta, path)
VALUES ($1, $2, $3, $4, $5, $6, 'placeholder')
RETURNING id, kind, external_id, name, parent_id, principal_id, path::text, meta, created_at;

-- name: GetScopeByID :one
SELECT id, kind, external_id, name, parent_id, principal_id, path::text, meta, created_at
FROM scopes WHERE id = $1;

-- name: GetScopeByExternalID :one
SELECT id, kind, external_id, name, parent_id, principal_id, path::text, meta, created_at
FROM scopes WHERE kind = $1 AND external_id = $2;

-- name: GetAncestorScopeIDs :many
SELECT s2.id FROM scopes s1
JOIN scopes s2 ON s2.path @> s1.path
WHERE s1.id = $1;
