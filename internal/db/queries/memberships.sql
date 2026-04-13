-- name: CreateMembership :one
INSERT INTO principal_memberships (member_id, parent_id, role, granted_by)
VALUES ($1, $2, $3, $4)
RETURNING member_id, parent_id, role, granted_by, created_at;

-- name: DeleteMembership :exec
DELETE FROM principal_memberships WHERE member_id = $1 AND parent_id = $2;

-- name: GetMemberships :many
SELECT member_id, parent_id, role, granted_by, created_at
FROM principal_memberships WHERE member_id = $1;

-- name: GetAllParentIDs :many
WITH RECURSIVE member_tree AS (
    SELECT $1::uuid AS id
    UNION ALL
    SELECT pm.parent_id
    FROM   principal_memberships pm
    JOIN   member_tree mt ON pm.member_id = mt.id
)
SELECT id FROM member_tree;

-- name: GetPrincipalIsSystemAdmin :one
SELECT is_system_admin FROM principals WHERE id = $1;

-- name: GetScopesForPrincipals :many
SELECT id FROM scopes WHERE principal_id = ANY($1::uuid[]);

-- name: IsScopeAdmin :one
SELECT EXISTS(
    SELECT 1 FROM scopes sc1
    WHERE sc1.id = ANY($1::uuid[]) AND sc1.principal_id = $2
    UNION ALL
    SELECT 1 FROM principal_memberships pm
    JOIN scopes sc2 ON sc2.principal_id = pm.parent_id
    WHERE sc2.id = ANY($1::uuid[])
    AND pm.member_id = $2
    AND pm.role = 'admin'
);

-- name: IsPrincipalAdmin :one
SELECT EXISTS(
    SELECT 1
    FROM principal_memberships pm
    WHERE pm.member_id = $1
    AND pm.parent_id = ANY($2::uuid[])
    AND pm.role = 'admin'
);

-- name: HasAnyAdminMembership :one
SELECT EXISTS(
    SELECT 1
    FROM principal_memberships pm
    WHERE pm.member_id = $1
    AND pm.role = 'admin'
);
