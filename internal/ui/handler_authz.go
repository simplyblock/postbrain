package ui

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/authz"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/principals"
)

// principalFromCookie resolves the principal ID from the pb_session cookie.
// Returns uuid.Nil if the cookie is missing or invalid.
func (h *Handler) principalFromCookie(r *http.Request) uuid.UUID {
	token := h.tokenFromCookie(r)
	if token == nil {
		return uuid.Nil
	}
	return token.PrincipalID
}

// tokenFromCookie resolves the current session token from the pb_session cookie.
// Returns nil if the cookie is missing or invalid.
func (h *Handler) tokenFromCookie(r *http.Request) *db.Token {
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" || h.pool == nil {
		return nil
	}
	hash := auth.HashToken(cookie.Value)
	token, err := auth.NewTokenStore(h.pool).Lookup(r.Context(), hash)
	if err != nil || token == nil {
		return nil
	}
	return token
}

func (h *Handler) hasScopeAdminAccess(ctx context.Context, r *http.Request, scopeID uuid.UUID) bool {
	return h.hasScopePermission(ctx, r, scopeID, authz.NewPermission(authz.ResourceScopes, authz.OperationEdit))
}

func (h *Handler) hasScopePermission(ctx context.Context, r *http.Request, scopeID uuid.UUID, permission authz.Permission) bool {
	if h.pool == nil {
		return false
	}
	token := h.tokenFromCookie(r)
	if token == nil || token.PrincipalID == uuid.Nil {
		return false
	}
	tokenResolver := authz.NewTokenResolver(authz.NewDBResolver(h.pool))
	ok, err := tokenResolver.HasTokenPermission(ctx, token, scopeID, permission)
	return err == nil && ok
}

func (h *Handler) hasPrincipalAdminAccess(ctx context.Context, r *http.Request, targetPrincipalID uuid.UUID) bool {
	if h.pool == nil {
		return false
	}
	token := h.tokenFromCookie(r)
	if token == nil || token.PrincipalID == uuid.Nil {
		return false
	}
	ms := principals.NewMembershipStore(h.pool)
	ok, err := ms.IsPrincipalAdmin(ctx, token.PrincipalID, targetPrincipalID)
	return err == nil && ok
}

func (h *Handler) hasAnyPrincipalAdminRole(ctx context.Context, r *http.Request) bool {
	if h.pool == nil {
		return false
	}
	token := h.tokenFromCookie(r)
	if token == nil || token.PrincipalID == uuid.Nil {
		return false
	}
	// System admins always have full access.
	if p, err := db.GetPrincipalByID(ctx, h.pool, token.PrincipalID); err == nil && p != nil && p.IsSystemAdmin {
		return true
	}
	ms := principals.NewMembershipStore(h.pool)
	ok, err := ms.HasAnyAdminRole(ctx, token.PrincipalID)
	return err == nil && ok
}

// authorizedScopesForRequest resolves scopes reachable by the current principal
// (via the authz resolver), intersected with token scope restrictions when
// scope_ids is non-nil.  Token restrictions are expanded to include ancestor
// scopes so a token scoped to a child scope can still read its parents.
func (h *Handler) authorizedScopesForRequest(ctx context.Context, r *http.Request) ([]*db.Scope, map[uuid.UUID]struct{}) {
	out := map[uuid.UUID]struct{}{}
	if h.pool == nil {
		return []*db.Scope{}, out
	}
	token := h.tokenFromCookie(r)
	if token == nil || token.PrincipalID == uuid.Nil {
		return []*db.Scope{}, out
	}
	ids, err := authz.NewDBResolver(h.pool).ReachableScopeIDs(ctx, token.PrincipalID)
	if err != nil {
		return []*db.Scope{}, out
	}
	if token.ScopeIds != nil {
		allowed := make(map[uuid.UUID]struct{}, len(token.ScopeIds))
		for _, id := range token.ScopeIds {
			allowed[id] = struct{}{}
			ancestorIDs, err := db.GetAncestorScopeIDs(ctx, h.pool, id)
			if err == nil {
				for _, ancestorID := range ancestorIDs {
					allowed[ancestorID] = struct{}{}
				}
			}
		}
		intersected := make([]uuid.UUID, 0, len(ids))
		for _, id := range ids {
			if _, ok := allowed[id]; ok {
				intersected = append(intersected, id)
			}
		}
		// If no effective scopes are resolved for the principal, fall back to
		// explicit token scope restrictions so scoped session tokens still work.
		if len(ids) == 0 {
			for id := range allowed {
				intersected = append(intersected, id)
			}
		}
		ids = intersected
	}
	scopes, err := db.GetScopesByIDs(ctx, h.pool, ids)
	if err != nil {
		return []*db.Scope{}, out
	}
	for _, s := range scopes {
		out[s.ID] = struct{}{}
	}
	return scopes, out
}

// effectivePrincipalScopesForRequest resolves all scopes reachable by the
// current principal via the authz resolver, without applying token scope
// restrictions.  Used for the token-creation form where you pick which scopes
// a new token should be limited to.
func (h *Handler) effectivePrincipalScopesForRequest(ctx context.Context, r *http.Request) ([]*db.Scope, map[uuid.UUID]struct{}) {
	out := map[uuid.UUID]struct{}{}
	if h.pool == nil {
		return []*db.Scope{}, out
	}
	token := h.tokenFromCookie(r)
	if token == nil || token.PrincipalID == uuid.Nil {
		return []*db.Scope{}, out
	}
	ids, err := authz.NewDBResolver(h.pool).ReachableScopeIDs(ctx, token.PrincipalID)
	if err != nil {
		return []*db.Scope{}, out
	}
	scopes, err := db.GetScopesByIDs(ctx, h.pool, ids)
	if err != nil {
		return []*db.Scope{}, out
	}
	for _, s := range scopes {
		out[s.ID] = struct{}{}
	}
	return scopes, out
}

// reachablePrincipalIDSet returns the set of principal IDs visible on the
// principals page for the current user. Returns nil to indicate "show all"
// (used for system admins). For non-sysadmin users it returns self + all
// ancestor principals + all direct members of groups the user admins.
func (h *Handler) reachablePrincipalIDSet(ctx context.Context, r *http.Request) map[uuid.UUID]struct{} {
	out := map[uuid.UUID]struct{}{}
	if h.pool == nil {
		return out
	}
	principalID := h.principalFromCookie(r)
	if principalID == uuid.Nil {
		return out
	}
	// System admins see everything.
	if p, err := db.GetPrincipalByID(ctx, h.pool, principalID); err == nil && p != nil && p.IsSystemAdmin {
		return nil
	}
	// Self + all ancestor principals.
	ids, err := db.GetAllParentIDs(ctx, h.pool, principalID)
	if err != nil {
		return out
	}
	for _, id := range ids {
		out[id] = struct{}{}
	}
	// Also include direct members of any group this principal admins.
	rows, err := h.pool.Query(ctx,
		`SELECT DISTINCT pm2.member_id
		 FROM principal_memberships pm
		 JOIN principal_memberships pm2 ON pm2.parent_id = pm.parent_id
		 WHERE pm.member_id = $1 AND pm.role = 'admin'`,
		principalID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			if rows.Scan(&id) == nil {
				out[id] = struct{}{}
			}
		}
	}
	return out
}
