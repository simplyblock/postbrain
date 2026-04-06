package authz

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Resolver computes effective permissions for a (principal, scope) pair.
type Resolver interface {
	// EffectivePermissions returns the union of all permissions the principal
	// holds on the given scope, including membership roles, direct grants,
	// downward inheritance from ancestor scopes, and upward read from
	// descendant scopes.
	EffectivePermissions(ctx context.Context, principalID, scopeID uuid.UUID) (PermissionSet, error)

	// HasPermission returns true if the principal holds the specified permission
	// on the given scope.
	HasPermission(ctx context.Context, principalID, scopeID uuid.UUID, perm Permission) (bool, error)
}

// DBResolver is a Resolver backed by a PostgreSQL connection pool.
type DBResolver struct {
	pool *pgxpool.Pool
}

// NewDBResolver creates a DBResolver using the given pool.
func NewDBResolver(pool *pgxpool.Pool) *DBResolver {
	return &DBResolver{pool: pool}
}

// EffectivePermissions resolves permissions for (principalID, scopeID) following
// the four-source algorithm:
//  1. systemadmin flag → all permissions
//  2. direct ownership (scope.principal_id == principalID) → RoleOwner
//  3. direct membership of principalID in scope's owning principal → role perms
//  4. direct scope grants (on scopeID and ancestor scopes) → grant perms
//  5. upward read: grants on descendant scopes → propagated :read on scopeID
//
// Sources 2-5 are unioned.
func (r *DBResolver) EffectivePermissions(ctx context.Context, principalID, scopeID uuid.UUID) (PermissionSet, error) {
	// 1. systemadmin check
	var isSystemAdmin bool
	err := r.pool.QueryRow(ctx,
		`SELECT is_system_admin FROM principals WHERE id = $1`,
		principalID,
	).Scan(&isSystemAdmin)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EmptyPermissionSet(), nil
		}
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: principal lookup: %w", err)
	}
	if isSystemAdmin {
		return newAllPermissions(), nil
	}

	// Ensure scope exists.
	var scopeExists bool
	err = r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM scopes WHERE id = $1)`,
		scopeID,
	).Scan(&scopeExists)
	if err != nil {
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: scope lookup: %w", err)
	}
	if !scopeExists {
		return EmptyPermissionSet(), nil
	}

	result := EmptyPermissionSet()

	// 2. Direct ownership on scopeID or any ancestor scope.
	var ownsTargetOrAncestor bool
	err = r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM scopes owner_scope
			JOIN scopes target ON owner_scope.path @> target.path
			WHERE target.id = $2
			  AND owner_scope.principal_id = $1
		)
	`, principalID, scopeID).Scan(&ownsTargetOrAncestor)
	if err != nil {
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: ownership lookup: %w", err)
	}
	if ownsTargetOrAncestor {
		result = Union(result, RolePermissions(RoleOwner))
	}

	// 3. Direct membership in the owning principal of scopeID (or any ancestor scope's owner).
	// We query for the best role the caller holds as a direct member of any scope-owning
	// principal in the ancestor chain (scope + its ancestors).
	var roleStr *string
	err = r.pool.QueryRow(ctx, `
		SELECT pm.role
		FROM principal_memberships pm
		JOIN (
			SELECT DISTINCT s.principal_id
			FROM scopes s
			JOIN scopes t ON s.path @> t.path
			WHERE t.id = $2
		) owners ON pm.parent_id = owners.principal_id
		WHERE pm.member_id = $1
		ORDER BY CASE pm.role
			WHEN 'owner'  THEN 1
			WHEN 'admin'  THEN 2
			WHEN 'member' THEN 3
			ELSE 4
		END
		LIMIT 1
	`, principalID, scopeID).Scan(&roleStr)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: membership lookup: %w", err)
	}
	if roleStr != nil {
		role, err := ParseRole(*roleStr)
		if err != nil {
			return EmptyPermissionSet(), fmt.Errorf("authz: resolver: invalid membership role %q: %w", *roleStr, err)
		}
		result = Union(result, RolePermissions(role))
	}

	// 4. Direct scope grants on scopeID and all ancestor scopes (downward inheritance).
	// A grant on an ancestor scope applies to all its descendants.
	rows, err := r.pool.Query(ctx, `
		SELECT sg.permissions
		FROM scope_grants sg
		JOIN scopes ancestor ON sg.scope_id = ancestor.id
		JOIN scopes target   ON ancestor.path @> target.path
		WHERE sg.principal_id = $1
		  AND target.id = $2
		  AND (sg.expires_at IS NULL OR sg.expires_at > now())
	`, principalID, scopeID)
	if err != nil {
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: scope grants lookup: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var rawPerms []string
		if err := rows.Scan(&rawPerms); err != nil {
			return EmptyPermissionSet(), fmt.Errorf("authz: resolver: scan grant permissions: %w", err)
		}
		ps, err := NewPermissionSet(rawPerms)
		if err != nil {
			return EmptyPermissionSet(), fmt.Errorf("authz: resolver: invalid scope grant permissions: %w", err)
		}
		result = Union(result, ps)
	}
	if err := rows.Err(); err != nil {
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: scope grants rows: %w", err)
	}

	// 5. Upward read from all grant sources:
	// ownership, membership-derived permissions, and direct scope grants.
	upwardRead := EmptyPermissionSet()

	var ownsDescendant bool
	err = r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM scopes target
			JOIN scopes owned ON target.path @> owned.path
			WHERE target.id = $1
			  AND owned.id != target.id
			  AND owned.principal_id = $2
		)
	`, scopeID, principalID).Scan(&ownsDescendant)
	if err != nil {
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: descendant ownership lookup: %w", err)
	}
	if ownsDescendant {
		upwardRead = Union(upwardRead, allReadPermissions())
	}

	memberRows, err := r.pool.Query(ctx, `
		SELECT pm.role
		FROM principal_memberships pm
		JOIN scopes owned  ON pm.parent_id = owned.principal_id
		JOIN scopes target ON target.path @> owned.path
		WHERE pm.member_id = $1
		  AND target.id = $2
		  AND owned.id != target.id
	`, principalID, scopeID)
	if err != nil {
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: descendant membership lookup: %w", err)
	}
	defer memberRows.Close()
	for memberRows.Next() {
		var rawRole string
		if err := memberRows.Scan(&rawRole); err != nil {
			return EmptyPermissionSet(), fmt.Errorf("authz: resolver: scan descendant membership role: %w", err)
		}
		role, err := ParseRole(rawRole)
		if err != nil {
			return EmptyPermissionSet(), fmt.Errorf("authz: resolver: invalid descendant membership role %q: %w", rawRole, err)
		}
		upwardRead = Union(upwardRead, readOnlyPermissionSet(RolePermissions(role)))
	}
	if err := memberRows.Err(); err != nil {
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: descendant membership rows: %w", err)
	}

	descRows, err := r.pool.Query(ctx, `
		SELECT sg.permissions
		FROM scope_grants sg
		JOIN scopes descendant ON sg.scope_id = descendant.id
		JOIN scopes target     ON target.path @> descendant.path
		WHERE sg.principal_id = $1
		  AND target.id = $2
		  AND descendant.id != target.id
		  AND (sg.expires_at IS NULL OR sg.expires_at > now())
	`, principalID, scopeID)
	if err != nil {
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: upward read lookup: %w", err)
	}
	defer descRows.Close()
	for descRows.Next() {
		var rawPerms []string
		if err := descRows.Scan(&rawPerms); err != nil {
			return EmptyPermissionSet(), fmt.Errorf("authz: resolver: scan upward perms: %w", err)
		}
		ps, err := NewPermissionSet(rawPerms)
		if err != nil {
			return EmptyPermissionSet(), fmt.Errorf("authz: resolver: invalid descendant scope grant permissions: %w", err)
		}
		upwardRead = Union(upwardRead, readOnlyPermissionSet(ps))
	}
	if err := descRows.Err(); err != nil {
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: upward read rows: %w", err)
	}

	result = Union(result, upwardRead)

	return result, nil
}

// HasPermission returns true if the principal holds perm on scopeID.
func (r *DBResolver) HasPermission(ctx context.Context, principalID, scopeID uuid.UUID, perm Permission) (bool, error) {
	perms, err := r.EffectivePermissions(ctx, principalID, scopeID)
	if err != nil {
		return false, err
	}
	return perms.Contains(perm), nil
}

// ReachableScopeIDs returns all scope IDs where principalID has any permissions.
// This includes:
//   - Scopes owned by the principal or any principal it is a member of.
//   - Scopes with a direct scope_grant for the principal.
//   - Ancestor scopes of any directly-accessible scope (upward-read inheritance).
//
// The result is suitable for filtering list queries.
func (r *DBResolver) ReachableScopeIDs(ctx context.Context, principalID uuid.UUID) ([]uuid.UUID, error) {
	// systemadmin: all scopes are accessible.
	var isSystemAdmin bool
	if err := r.pool.QueryRow(ctx,
		`SELECT is_system_admin FROM principals WHERE id = $1`, principalID,
	).Scan(&isSystemAdmin); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("authz: reachable scopes: principal lookup: %w", err)
	}
	if isSystemAdmin {
		rows, err := r.pool.Query(ctx, `SELECT id FROM scopes`)
		if err != nil {
			return nil, fmt.Errorf("authz: reachable scopes: all scopes: %w", err)
		}
		defer rows.Close()
		var ids []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, rows.Err()
	}

	rows, err := r.pool.Query(ctx, `
		WITH
		-- All principals the caller belongs to (self + all ancestor principals via membership).
		principal_ancestry AS (
			SELECT $1::uuid AS id
			UNION
			SELECT parent_id FROM principal_memberships WHERE member_id = $1
		),
		-- Scopes accessible via ownership or membership (downward inheritance implied).
		membership_scopes AS (
			SELECT id FROM scopes WHERE principal_id IN (SELECT id FROM principal_ancestry)
		),
		-- Scopes with a direct non-expired grant for the principal.
		grant_scopes AS (
			SELECT DISTINCT scope_id AS id
			FROM scope_grants
			WHERE principal_id = $1
			  AND (expires_at IS NULL OR expires_at > now())
		),
		-- Union of directly accessible scopes.
		direct_scopes AS (
			SELECT id FROM membership_scopes
			UNION
			SELECT id FROM grant_scopes
		),
		-- Ancestors of directly accessible scopes (upward-read: seeing a child implies reading the parent).
		upward_scopes AS (
			SELECT DISTINCT ancestor.id
			FROM scopes ancestor
			JOIN scopes direct ON ancestor.path @> direct.path AND ancestor.id != direct.id
			WHERE direct.id IN (SELECT id FROM grant_scopes)
		)
		SELECT id FROM direct_scopes
		UNION
		SELECT id FROM upward_scopes
	`, principalID)
	if err != nil {
		return nil, fmt.Errorf("authz: reachable scopes: query: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("authz: reachable scopes: scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// newAllPermissions builds a PermissionSet containing every valid permission.
func newAllPermissions() PermissionSet {
	all := AllPermissions()
	raw := make([]string, len(all))
	for i, p := range all {
		raw[i] = string(p)
	}
	ps, _ := NewPermissionSet(raw)
	return ps
}

// newSinglePermission builds a PermissionSet from a single Permission.
func newSinglePermission(p Permission) PermissionSet {
	ps, _ := NewPermissionSet([]string{string(p)})
	return ps
}

func readOnlyPermissionSet(ps PermissionSet) PermissionSet {
	out := EmptyPermissionSet()
	for _, p := range ps.Permissions() {
		_, op, err := p.Parse()
		if err != nil {
			continue
		}
		if op == OperationRead {
			out = Union(out, newSinglePermission(p))
		}
	}
	return out
}

func allReadPermissions() PermissionSet {
	return readOnlyPermissionSet(newAllPermissions())
}
