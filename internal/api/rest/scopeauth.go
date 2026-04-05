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
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/metrics"
)

func (ro *Router) authorizeRequestedScope(ctx context.Context, requestedScopeID uuid.UUID) error {
	return scopeauth.AuthorizeContextScope(ctx, ro.membership, requestedScopeID)
}

func (ro *Router) authorizeObjectScope(ctx context.Context, objectScopeID uuid.UUID) error {
	return ro.authorizeRequestedScope(ctx, objectScopeID)
}

func (ro *Router) effectiveScopeIDsForRequest(ctx context.Context) ([]uuid.UUID, error) {
	if ids, ok := scopeauth.EffectiveScopeIDsFromContext(ctx); ok {
		return ids, nil
	}
	if ro.membership == nil {
		return nil, nil
	}
	principalID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	if principalID == uuid.Nil {
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
	if token == nil || token.ScopeIds == nil {
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
