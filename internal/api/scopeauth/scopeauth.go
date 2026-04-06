package scopeauth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/authz"
	"github.com/simplyblock/postbrain/internal/db"
)

var (
	// ErrTokenScopeDenied indicates the requested scope is outside token scope_ids or
	// the token lacks the required permission on the scope.
	ErrTokenScopeDenied = errors.New("scopeauth: token scope denied")
	// ErrPrincipalScopeDenied indicates the requested scope is outside principal effective scopes.
	ErrPrincipalScopeDenied = errors.New("scopeauth: principal scope denied")
	// ErrMissingToken indicates the request context has no authenticated token.
	ErrMissingToken = errors.New("scopeauth: missing token in context")
	// ErrMissingPrincipal indicates the request context has no principal ID.
	ErrMissingPrincipal = errors.New("scopeauth: missing principal in context")
	// ErrScopeResolverUnavailable indicates effective-scope resolution is unavailable.
	ErrScopeResolverUnavailable = errors.New("scopeauth: effective scope resolver unavailable")
)

// AuthorizeContextScope enforces scope access using the authz.TokenResolver injected
// into the request context by the auth middleware. It verifies that the authenticated
// token holds the specified permission on the given scope, enforcing both:
//   - token scope_ids restrictions (if declared), and
//   - the principal's effective permissions on that scope (ownership, membership,
//     direct grants, and inheritance).
//
// perm must be the specific permission the caller needs (e.g. "memories:write").
func AuthorizeContextScope(ctx context.Context, requestedScopeID uuid.UUID, perm authz.Permission) error {
	token, _ := ctx.Value(auth.ContextKeyToken).(*db.Token)
	if token == nil {
		return fmt.Errorf("%w", ErrMissingToken)
	}

	tokenResolver, _ := ctx.Value(auth.ContextKeyTokenResolver).(*authz.TokenResolver)
	if tokenResolver == nil {
		return fmt.Errorf("%w", ErrScopeResolverUnavailable)
	}

	ok, err := tokenResolver.HasTokenPermission(ctx, token, requestedScopeID, perm)
	if err != nil {
		return fmt.Errorf("scopeauth: check permission: %w", err)
	}
	if !ok {
		return fmt.Errorf("%w: token lacks %s on scope %s", ErrTokenScopeDenied, perm, requestedScopeID)
	}
	return nil
}
