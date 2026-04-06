package authz

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
)

// TokenResolver wraps a Resolver to add token-level permission and scope restrictions.
type TokenResolver struct {
	resolver Resolver
}

// NewTokenResolver creates a TokenResolver backed by the given Resolver.
func NewTokenResolver(r Resolver) *TokenResolver {
	return &TokenResolver{resolver: r}
}

// EffectiveTokenPermissions returns the permissions the token holds on the
// given scope. It enforces two axes of restriction:
//  1. Permission restriction: intersection of principal's effective permissions
//     and the token's declared permissions.
//  2. Scope restriction: if the token has scope_ids set, the requested scope
//     must be a descendant of (or equal to) one of the declared scopes.
//     If scope_ids is empty, no scope restriction applies.
//
// Returns an empty set for revoked or expired tokens.
func (tr *TokenResolver) EffectiveTokenPermissions(ctx context.Context, tok *db.Token, scopeID uuid.UUID) (PermissionSet, error) {
	// Guard: revoked or expired tokens have no permissions.
	if tok.RevokedAt != nil {
		return EmptyPermissionSet(), nil
	}
	if tok.ExpiresAt != nil && tok.ExpiresAt.Before(time.Now()) {
		return EmptyPermissionSet(), nil
	}

	// Scope restriction: if token has declared scope_ids, verify the requested
	// scope is in the allowed set (or is a descendant of an allowed scope).
	if len(tok.ScopeIds) > 0 {
		allowed, err := tr.isScopeAllowed(ctx, scopeID, tok.ScopeIds)
		if err != nil {
			return EmptyPermissionSet(), err
		}
		if !allowed {
			return EmptyPermissionSet(), nil
		}
	}

	// Principal effective permissions on this scope.
	principalPerms, err := tr.resolver.EffectivePermissions(ctx, tok.PrincipalID, scopeID)
	if err != nil {
		return EmptyPermissionSet(), err
	}

	// Token declared permissions (expanded from raw strings).
	tokenPerms, err := ParseTokenPermissions(tok.Permissions)
	if err != nil {
		return EmptyPermissionSet(), err
	}

	return EffectiveTokenPermissions(principalPerms, tokenPerms), nil
}

// HasTokenPermission returns true if the token holds perm on scopeID.
func (tr *TokenResolver) HasTokenPermission(ctx context.Context, tok *db.Token, scopeID uuid.UUID, perm Permission) (bool, error) {
	perms, err := tr.EffectiveTokenPermissions(ctx, tok, scopeID)
	if err != nil {
		return false, err
	}
	return perms.Contains(perm), nil
}

// DBResolver returns the underlying *DBResolver if one is present, or nil.
// This allows callers to access bulk operations like ReachableScopeIDs.
func (tr *TokenResolver) DBResolver() *DBResolver {
	dbr, _ := unwrapDBResolver(tr.resolver)
	return dbr
}

// isScopeAllowed returns true if scopeID equals or is a descendant of any
// scope in allowedIDs. Uses the ltree ancestry relationship stored in scopes.path.
func (tr *TokenResolver) isScopeAllowed(ctx context.Context, scopeID uuid.UUID, allowedIDs []uuid.UUID) (bool, error) {
	dbResolver, ok := unwrapDBResolver(tr.resolver)
	if !ok {
		return false, fmt.Errorf("authz: token resolver requires DB-backed resolver for scope restrictions")
	}

	// The requested scope is allowed if any of the token's declared scope_ids
	// is an ancestor of (or equal to) the requested scope.
	// Using ltree: allowed.path @> target.path means allowed is ancestor of target.
	var allowed bool
	err := dbResolver.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM scopes target
			JOIN scopes allowed ON allowed.path @> target.path
			WHERE target.id = $1
			  AND allowed.id = ANY($2)
		)
	`, scopeID, allowedIDs).Scan(&allowed)
	if err != nil {
		return false, err
	}
	return allowed, nil
}

func unwrapDBResolver(r Resolver) (*DBResolver, bool) {
	switch v := r.(type) {
	case *DBResolver:
		return v, true
	case *CachedResolver:
		return unwrapDBResolver(v.inner)
	default:
		return nil, false
	}
}
