-- name: GetPrincipalSystemAdmin :one
SELECT is_system_admin FROM principals WHERE id = $1;

-- name: ScopeExists :one
SELECT EXISTS(SELECT 1 FROM scopes WHERE id = $1);

-- name: GetAllScopeIDs :many
SELECT id FROM scopes;

-- name: PrincipalOwnsTargetOrAncestor :one
SELECT EXISTS (
    SELECT 1
    FROM scopes owner_scope
    JOIN scopes target ON owner_scope.path @> target.path
    WHERE target.id = $2
      AND owner_scope.principal_id = $1
);

-- name: GetEffectiveMembershipRole :one
WITH RECURSIVE principal_ancestry(id) AS (
    SELECT $1::uuid
    UNION
    SELECT pm2.parent_id
    FROM principal_memberships pm2
    JOIN principal_ancestry pa ON pm2.member_id = pa.id
)
SELECT pm.role
FROM principal_memberships pm
JOIN (
    SELECT DISTINCT s.principal_id
    FROM scopes s
    JOIN scopes t ON s.path @> t.path
    WHERE t.id = $2
) owners ON pm.parent_id = owners.principal_id
WHERE pm.member_id IN (SELECT id FROM principal_ancestry)
ORDER BY CASE pm.role
    WHEN 'owner'  THEN 1
    WHEN 'admin'  THEN 2
    WHEN 'member' THEN 3
    ELSE 4
END
LIMIT 1;

-- name: GetScopeGrantPermissions :many
SELECT sg.permissions
FROM scope_grants sg
JOIN scopes ancestor ON sg.scope_id = ancestor.id
JOIN scopes target   ON ancestor.path @> target.path
WHERE sg.principal_id = $1
  AND target.id = $2
  AND (sg.expires_at IS NULL OR sg.expires_at > now());

-- name: PrincipalOwnsDescendant :one
SELECT EXISTS (
    SELECT 1
    FROM scopes target
    JOIN scopes owned ON target.path @> owned.path
    WHERE target.id = $1
      AND owned.id != target.id
      AND owned.principal_id = $2
);

-- name: GetDescendantMembershipRoles :many
WITH RECURSIVE principal_ancestry(id) AS (
    SELECT $1::uuid
    UNION
    SELECT pm2.parent_id
    FROM principal_memberships pm2
    JOIN principal_ancestry pa ON pm2.member_id = pa.id
)
SELECT pm.role
FROM principal_memberships pm
JOIN scopes owned  ON pm.parent_id = owned.principal_id
JOIN scopes target ON target.path @> owned.path
WHERE pm.member_id IN (SELECT id FROM principal_ancestry)
  AND target.id = $2
  AND owned.id != target.id;

-- name: GetDescendantScopeGrantPermissions :many
SELECT sg.permissions
FROM scope_grants sg
JOIN scopes descendant ON sg.scope_id = descendant.id
JOIN scopes target     ON target.path @> descendant.path
WHERE sg.principal_id = $1
  AND target.id = $2
  AND descendant.id != target.id
  AND (sg.expires_at IS NULL OR sg.expires_at > now());

-- name: GetReachableScopeIDs :many
WITH RECURSIVE
principal_ancestry AS (
    SELECT $1::uuid AS id
    UNION
    SELECT pm.parent_id
    FROM principal_memberships pm
    JOIN principal_ancestry pa ON pm.member_id = pa.id
),
membership_scopes AS (
    SELECT id FROM scopes WHERE principal_id IN (SELECT id FROM principal_ancestry)
),
grant_scopes AS (
    SELECT DISTINCT scope_id AS id
    FROM scope_grants
    WHERE principal_id = $1
      AND (expires_at IS NULL OR expires_at > now())
),
direct_scopes AS (
    SELECT id FROM membership_scopes
    UNION
    SELECT id FROM grant_scopes
),
upward_scopes AS (
    SELECT DISTINCT ancestor.id
    FROM scopes ancestor
    JOIN scopes direct ON ancestor.path @> direct.path AND ancestor.id != direct.id
    WHERE direct.id IN (SELECT id FROM direct_scopes)
)
SELECT id FROM direct_scopes
UNION
SELECT id FROM upward_scopes;
