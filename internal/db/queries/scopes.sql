-- name: CreateScope :one
INSERT INTO scopes (kind, external_id, name, parent_id, principal_id, meta, path)
VALUES ($1, $2, $3, NULLIF($4, '00000000-0000-0000-0000-000000000000'::uuid), $5, $6, 'placeholder')
RETURNING id, kind, external_id, name, parent_id, principal_id, path::text, meta,
          repo_url, repo_default_branch, last_indexed_commit, created_at;

-- name: GetScopeByID :one
SELECT id, kind, external_id, name, parent_id, principal_id, path::text, meta,
       repo_url, repo_default_branch, last_indexed_commit, created_at
FROM scopes WHERE id = $1;

-- name: GetScopeByExternalID :one
SELECT id, kind, external_id, name, parent_id, principal_id, path::text, meta,
       repo_url, repo_default_branch, last_indexed_commit, created_at
FROM scopes WHERE kind = $1 AND external_id = $2;

-- name: GetAncestorScopeIDs :many
SELECT s2.id FROM scopes s1
JOIN scopes s2 ON s2.path @> s1.path
WHERE s1.id = $1;

-- name: ListScopes :many
SELECT id, kind, external_id, name, parent_id, principal_id, path::text, meta,
       repo_url, repo_default_branch, last_indexed_commit, created_at
FROM scopes
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdateScope :one
UPDATE scopes SET name = $2, meta = $3 WHERE id = $1
RETURNING id, kind, external_id, name, parent_id, principal_id, path::text, meta,
          repo_url, repo_default_branch, last_indexed_commit, created_at;

-- name: UpdateScopeOwner :one
UPDATE scopes SET principal_id = $2 WHERE id = $1
RETURNING id, kind, external_id, name, parent_id, principal_id, path::text, meta,
          repo_url, repo_default_branch, last_indexed_commit, created_at;

-- name: SetScopeRepo :one
UPDATE scopes
SET repo_url = $2, repo_default_branch = $3
WHERE id = $1 AND kind = 'project'
RETURNING id, kind, external_id, name, parent_id, principal_id, path::text, meta,
          repo_url, repo_default_branch, last_indexed_commit, created_at;

-- name: SetLastIndexedCommit :exec
UPDATE scopes SET last_indexed_commit = $2 WHERE id = $1;

-- name: CountChildScopes :one
SELECT COUNT(*) FROM scopes WHERE parent_id = $1;

-- name: DeleteScope :exec
DELETE FROM scopes WHERE id = $1;
