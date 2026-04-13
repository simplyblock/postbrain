package authz

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
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
	q := db.New(r.pool)

	// 1. systemadmin check
	isSystemAdmin, err := q.GetPrincipalSystemAdmin(ctx, principalID)
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
	scopeExists, err := q.ScopeExists(ctx, scopeID)
	if err != nil {
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: scope lookup: %w", err)
	}
	if !scopeExists {
		return EmptyPermissionSet(), nil
	}

	result := EmptyPermissionSet()

	// 2. Direct ownership on scopeID or any ancestor scope.
	ownsTargetOrAncestor, err := q.PrincipalOwnsTargetOrAncestor(ctx, db.PrincipalOwnsTargetOrAncestorParams{
		PrincipalID: principalID,
		ID:          scopeID, // target scope
	})
	if err != nil {
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: ownership lookup: %w", err)
	}
	if ownsTargetOrAncestor {
		result = Union(result, RolePermissions(RoleOwner))
	}

	// 3. Membership derivation (full transitive chain via recursive CTE).
	roleStr, err := q.GetEffectiveMembershipRole(ctx, db.GetEffectiveMembershipRoleParams{
		Column1: principalID,
		ID:      scopeID,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: membership lookup: %w", err)
	}
	if err == nil {
		role, err := ParseRole(roleStr)
		if err != nil {
			return EmptyPermissionSet(), fmt.Errorf("authz: resolver: invalid membership role %q: %w", roleStr, err)
		}
		result = Union(result, RolePermissions(role))
	}

	// 4. Direct scope grants on scopeID and all ancestor scopes (downward inheritance).
	grantPerms, err := q.GetScopeGrantPermissions(ctx, db.GetScopeGrantPermissionsParams{
		PrincipalID: principalID,
		ID:          scopeID,
	})
	if err != nil {
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: scope grants lookup: %w", err)
	}
	for _, rawPerms := range grantPerms {
		ps, err := NewPermissionSet(rawPerms)
		if err != nil {
			return EmptyPermissionSet(), fmt.Errorf("authz: resolver: invalid scope grant permissions: %w", err)
		}
		result = Union(result, ps)
	}

	// 5. Upward read from all grant sources:
	// ownership, membership-derived permissions, and direct scope grants.
	upwardRead := EmptyPermissionSet()

	ownsDescendant, err := q.PrincipalOwnsDescendant(ctx, db.PrincipalOwnsDescendantParams{
		ID:          scopeID,
		PrincipalID: principalID,
	})
	if err != nil {
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: descendant ownership lookup: %w", err)
	}
	if ownsDescendant {
		upwardRead = Union(upwardRead, allReadPermissions())
	}

	descRoles, err := q.GetDescendantMembershipRoles(ctx, db.GetDescendantMembershipRolesParams{
		Column1: principalID,
		ID:      scopeID,
	})
	if err != nil {
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: descendant membership lookup: %w", err)
	}
	for _, rawRole := range descRoles {
		role, err := ParseRole(rawRole)
		if err != nil {
			return EmptyPermissionSet(), fmt.Errorf("authz: resolver: invalid descendant membership role %q: %w", rawRole, err)
		}
		upwardRead = Union(upwardRead, readOnlyPermissionSet(RolePermissions(role)))
	}

	descGrantPerms, err := q.GetDescendantScopeGrantPermissions(ctx, db.GetDescendantScopeGrantPermissionsParams{
		PrincipalID: principalID,
		ID:          scopeID,
	})
	if err != nil {
		return EmptyPermissionSet(), fmt.Errorf("authz: resolver: upward read lookup: %w", err)
	}
	for _, rawPerms := range descGrantPerms {
		ps, err := NewPermissionSet(rawPerms)
		if err != nil {
			return EmptyPermissionSet(), fmt.Errorf("authz: resolver: invalid descendant scope grant permissions: %w", err)
		}
		upwardRead = Union(upwardRead, readOnlyPermissionSet(ps))
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
	q := db.New(r.pool)

	// systemadmin: all scopes are accessible.
	isSystemAdmin, err := q.GetPrincipalSystemAdmin(ctx, principalID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("authz: reachable scopes: principal lookup: %w", err)
	}
	if isSystemAdmin {
		return q.GetAllScopeIDs(ctx)
	}

	return q.GetReachableScopeIDs(ctx, principalID)
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
