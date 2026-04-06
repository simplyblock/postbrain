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

	// 2. Direct ownership: scope.principal_id == principalID
	var ownerPrincipalID uuid.UUID
	err = r.pool.QueryRow(ctx,
		`SELECT principal_id FROM scopes WHERE id = $1`,
		scopeID,
	).Scan(&ownerPrincipalID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EmptyPermissionSet(), nil
		}
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: scope lookup: %w", err)
	}

	result := EmptyPermissionSet()

	if ownerPrincipalID == principalID {
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
		if role, err := ParseRole(*roleStr); err == nil {
			result = Union(result, RolePermissions(role))
		}
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
			continue // skip malformed grants silently
		}
		result = Union(result, ps)
	}
	if err := rows.Err(); err != nil {
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: scope grants rows: %w", err)
	}

	// 5. Upward read: grants on descendant scopes propagate matching :read
	// permissions upward to scopeID.
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
			continue
		}
		// Only propagate :read operations
		for _, p := range ps.Permissions() {
			_, op, err := p.Parse()
			if err == nil && op == OperationRead {
				result = Union(result, newSinglePermission(p))
			}
		}
	}
	if err := descRows.Err(); err != nil {
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: upward read rows: %w", err)
	}

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
