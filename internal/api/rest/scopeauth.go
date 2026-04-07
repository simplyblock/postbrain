package rest

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/api/scopeauth"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/authz"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/metrics"
)

// authorizeRequestedScope enforces scope access for the current request.
// It reads the route's required permission from context (set by permissionAuthzMiddleware)
// and calls AuthorizeContextScope with the TokenResolver injected by the auth middleware.
func (ro *Router) authorizeRequestedScope(ctx context.Context, requestedScopeID uuid.UUID) error {
	perm, _ := ctx.Value(contextKeyRoutePermission{}).(authz.Permission)
	if perm == "" {
		// No route permission entry — conservative default.
		perm = "scopes:read"
	}
	return scopeauth.AuthorizeContextScope(ctx, requestedScopeID, perm)
}

func (ro *Router) authorizeObjectScope(ctx context.Context, objectScopeID uuid.UUID) error {
	return ro.authorizeRequestedScope(ctx, objectScopeID)
}

// authorizeDeleteObjectScope enforces delete semantics: a caller may only delete
// objects in scopes directly owned by the caller principal (never in ancestor scopes).
func (ro *Router) authorizeDeleteObjectScope(ctx context.Context, objectScopeID uuid.UUID) error {
	if err := ro.authorizeRequestedScope(ctx, objectScopeID); err != nil {
		return err
	}

	scope, err := db.GetScopeByID(ctx, ro.pool, objectScopeID)
	if err != nil {
		return err
	}
	if scope == nil {
		return fmt.Errorf("%w: scope not found", scopeauth.ErrPrincipalScopeDenied)
	}

	principalID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	if principalID == uuid.Nil {
		return fmt.Errorf("%w", scopeauth.ErrMissingPrincipal)
	}
	if scope.PrincipalID != principalID {
		return fmt.Errorf("%w: delete not allowed in ancestor scope", scopeauth.ErrPrincipalScopeDenied)
	}
	return nil
}

func (ro *Router) authorizeScopeAdmin(ctx context.Context, scopeID uuid.UUID) error {
	if err := ro.authorizeRequestedScope(ctx, scopeID); err != nil {
		return err
	}
	principalID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	if principalID == uuid.Nil {
		return fmt.Errorf("%w", scopeauth.ErrMissingPrincipal)
	}

	// Prefer the DB resolver — it checks is_system_admin, ownership, membership
	// roles (admin+), and scope grants, so system admins are handled correctly.
	tokenResolver, _ := ctx.Value(auth.ContextKeyTokenResolver).(*authz.TokenResolver)
	if tokenResolver != nil {
		if dbr := tokenResolver.DBResolver(); dbr != nil {
			ok, err := dbr.HasPermission(ctx, principalID, scopeID, authz.NewPermission(authz.ResourceScopes, authz.OperationEdit))
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("%w: scope admin required", scopeauth.ErrPrincipalScopeDenied)
			}
			return nil
		}
	}

	// Fallback: membership-only admin check.
	if ro.membership == nil {
		return fmt.Errorf("%w", scopeauth.ErrScopeResolverUnavailable)
	}
	ok, err := ro.membership.IsScopeAdmin(ctx, principalID, scopeID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: scope admin required", scopeauth.ErrPrincipalScopeDenied)
	}
	return nil
}

// effectiveScopeIDsForRequest returns all scope IDs accessible to the principal,
// using the authz.DBResolver when available (which includes scope grants and
// upward-read inheritance), falling back to the membership store otherwise.
func (ro *Router) effectiveScopeIDsForRequest(ctx context.Context) ([]uuid.UUID, error) {
	principalID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	if principalID == uuid.Nil {
		return nil, nil
	}

	// Use the DB resolver's ReachableScopeIDs when available — it covers scope
	// grants and upward-read inheritance in addition to membership.
	tokenResolver, _ := ctx.Value(auth.ContextKeyTokenResolver).(*authz.TokenResolver)
	if tokenResolver != nil {
		if dbr := tokenResolver.DBResolver(); dbr != nil {
			return dbr.ReachableScopeIDs(ctx, principalID)
		}
	}

	// Fallback: membership-only scope resolution.
	if ro.membership == nil {
		return nil, nil
	}
	return ro.membership.EffectiveScopeIDs(ctx, principalID)
}

func (ro *Router) authorizedScopeIDsForRequest(ctx context.Context) ([]uuid.UUID, error) {
	effectiveScopeIDs, err := ro.effectiveScopeIDsForRequest(ctx)
	if err != nil {
		return nil, err
	}
	token, _ := ctx.Value(auth.ContextKeyToken).(*db.Token)
	if token == nil || len(token.ScopeIds) == 0 {
		return effectiveScopeIDs, nil
	}
	allowedByToken := make(map[uuid.UUID]struct{}, len(token.ScopeIds))
	for _, id := range token.ScopeIds {
		allowedByToken[id] = struct{}{}
	}
	authorized := make([]uuid.UUID, 0, len(effectiveScopeIDs))
	for _, id := range effectiveScopeIDs {
		if _, ok := allowedByToken[id]; ok {
			authorized = append(authorized, id)
		}
	}
	return authorized, nil
}

func writeScopeAuthzError(w http.ResponseWriter, r *http.Request, requestedScopeID uuid.UUID, err error) {
	switch {
	case errors.Is(err, scopeauth.ErrTokenScopeDenied),
		errors.Is(err, scopeauth.ErrPrincipalScopeDenied),
		errors.Is(err, scopeauth.ErrMissingToken),
		errors.Is(err, scopeauth.ErrMissingPrincipal):
		logScopeAuthzDenied(r.Context(), restEndpointLabel(r), requestedScopeID)
		writeError(w, http.StatusForbidden, "forbidden: scope access denied")
	default:
		writeError(w, http.StatusInternalServerError, "scope authorization failed")
	}
}

func restEndpointLabel(r *http.Request) string {
	if r == nil {
		return "unknown"
	}
	if rc := chi.RouteContext(r.Context()); rc != nil {
		if p := rc.RoutePattern(); p != "" {
			return fmt.Sprintf("%s %s", r.Method, p)
		}
	}
	return fmt.Sprintf("%s %s", r.Method, r.URL.Path)
}

func logScopeAuthzDenied(ctx context.Context, endpoint string, requestedScopeID uuid.UUID) {
	logger := LogFromContext(ctx)
	fields := []any{
		"surface", "rest",
		"endpoint", endpoint,
		"requested_scope_id", requestedScopeID,
	}
	if principalID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID); principalID != uuid.Nil {
		fields = append(fields, "principal_id", principalID)
	}
	if token, _ := ctx.Value(auth.ContextKeyToken).(*db.Token); token != nil {
		fields = append(fields, "token_id", token.ID)
	}
	logger.WarnContext(ctx, "scope access denied", fields...)
	metrics.ScopeAuthzDenied.WithLabelValues("rest", endpoint).Inc()
}
