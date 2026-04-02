package rest

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/api/scopeauth"
)

func (ro *Router) authorizeRequestedScope(ctx context.Context, requestedScopeID uuid.UUID) error {
	return scopeauth.AuthorizeContextScope(ctx, ro.membership, requestedScopeID)
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
