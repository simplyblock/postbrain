-- name: CreateSharingGrant :one
INSERT INTO sharing_grants (memory_id, artifact_id, grantee_scope_id, granted_by, can_reshare, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, memory_id, artifact_id, grantee_scope_id, granted_by, can_reshare, expires_at, created_at;

-- name: GetSharingGrant :one
SELECT id, memory_id, artifact_id, grantee_scope_id, granted_by, can_reshare, expires_at, created_at
FROM sharing_grants WHERE id = $1;

-- name: RevokeSharingGrant :exec
DELETE FROM sharing_grants WHERE id = $1;

-- name: ListSharingGrants :many
SELECT id, memory_id, artifact_id, grantee_scope_id, granted_by, can_reshare, expires_at, created_at
FROM sharing_grants WHERE grantee_scope_id = $1
ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: IsMemoryGranted :one
SELECT EXISTS (
    SELECT 1 FROM sharing_grants
    WHERE memory_id = $1
      AND grantee_scope_id = $2
      AND (expires_at IS NULL OR expires_at > now())
);

-- name: IsArtifactGranted :one
SELECT EXISTS (
    SELECT 1 FROM sharing_grants
    WHERE artifact_id = $1
      AND grantee_scope_id = $2
      AND (expires_at IS NULL OR expires_at > now())
);
