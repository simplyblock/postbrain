package rest

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/api/scopeauth"
	"github.com/simplyblock/postbrain/internal/auth"
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

func writeScopeAuthzError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, scopeauth.ErrTokenScopeDenied),
		errors.Is(err, scopeauth.ErrPrincipalScopeDenied),
		errors.Is(err, scopeauth.ErrMissingToken),
		errors.Is(err, scopeauth.ErrMissingPrincipal):
		writeError(w, http.StatusForbidden, "forbidden: scope access denied")
	default:
		writeError(w, http.StatusInternalServerError, "scope authorization failed")
	}
}
