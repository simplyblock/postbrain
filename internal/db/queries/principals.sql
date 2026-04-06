-- name: CreatePrincipal :one
INSERT INTO principals (kind, slug, display_name, meta)
VALUES ($1, $2, $3, $4)
RETURNING id, kind, slug, display_name, meta, created_at, updated_at, is_system_admin;

-- name: GetPrincipalByID :one
SELECT id, kind, slug, display_name, meta, created_at, updated_at, is_system_admin
FROM principals WHERE id = $1;

-- name: GetPrincipalBySlug :one
SELECT id, kind, slug, display_name, meta, created_at, updated_at, is_system_admin
FROM principals WHERE slug = $1;

-- name: UpdatePrincipal :one
UPDATE principals SET display_name=$2, meta=$3, updated_at=now()
WHERE id=$1
RETURNING id, kind, slug, display_name, meta, created_at, updated_at, is_system_admin;

-- name: DeletePrincipal :exec
DELETE FROM principals WHERE id = $1;

-- name: ListPrincipals :many
SELECT id, kind, slug, display_name, meta, created_at, updated_at, is_system_admin
FROM principals
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: SetSystemAdmin :exec
UPDATE principals SET is_system_admin = $2, updated_at = now()
WHERE id = $1;
