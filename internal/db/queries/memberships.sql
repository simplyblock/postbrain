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
