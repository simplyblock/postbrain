package scopeauth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
)

type contextKey string

const contextKeyEffectiveScopeIDs contextKey = "scopeauth_effective_scope_ids"

var (
	// ErrTokenScopeDenied indicates the requested scope is outside token scope_ids.
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

// EffectiveScopeResolver resolves scopes accessible to a principal (including multi-hop ancestry).
type EffectiveScopeResolver interface {
	EffectiveScopeIDs(ctx context.Context, principalID uuid.UUID) ([]uuid.UUID, error)
}

// AuthorizeRequestedScope enforces both scope gates:
// 1) token scope_ids restrictions (if present), and
// 2) principal effective-scope restrictions (must include requested scope).
func AuthorizeRequestedScope(token *db.Token, requestedScopeID uuid.UUID, effectiveScopeIDs []uuid.UUID) error {
	if token == nil {
		return fmt.Errorf("%w: missing token in context", ErrTokenScopeDenied)
	}
	if err := auth.EnforceScopeAccess(token, requestedScopeID); err != nil {
		return fmt.Errorf("%w: %v", ErrTokenScopeDenied, err)
	}
	for _, sid := range effectiveScopeIDs {
		if sid == requestedScopeID {
			return nil
		}
	}
	return fmt.Errorf("%w: requested scope %s not in principal effective scope set", ErrPrincipalScopeDenied, requestedScopeID)
}

// AuthorizeContextScope enforces scope access by reading auth values from context
// and resolving principal effective scope IDs via resolver.
func AuthorizeContextScope(ctx context.Context, resolver EffectiveScopeResolver, requestedScopeID uuid.UUID) error {
	if resolver == nil {
		return fmt.Errorf("%w", ErrScopeResolverUnavailable)
	}

	token, _ := ctx.Value(auth.ContextKeyToken).(*db.Token)
	if token == nil {
		return fmt.Errorf("%w", ErrMissingToken)
	}

	principalID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	if principalID == uuid.Nil {
		return fmt.Errorf("%w", ErrMissingPrincipal)
	}

	effectiveScopeIDs, ok := EffectiveScopeIDsFromContext(ctx)
	if !ok {
		var err error
		effectiveScopeIDs, err = resolver.EffectiveScopeIDs(ctx, principalID)
		if err != nil {
			return fmt.Errorf("scopeauth: resolve effective scopes: %w", err)
		}
	}
	return AuthorizeRequestedScope(token, requestedScopeID, effectiveScopeIDs)
}

// WithEffectiveScopeIDs stores resolved effective scope IDs in context.
func WithEffectiveScopeIDs(ctx context.Context, ids []uuid.UUID) context.Context {
	copied := append([]uuid.UUID(nil), ids...)
	return context.WithValue(ctx, contextKeyEffectiveScopeIDs, copied)
}

// EffectiveScopeIDsFromContext returns cached effective scope IDs when present.
func EffectiveScopeIDsFromContext(ctx context.Context) ([]uuid.UUID, bool) {
	ids, ok := ctx.Value(contextKeyEffectiveScopeIDs).([]uuid.UUID)
	if !ok {
		return nil, false
	}
	return append([]uuid.UUID(nil), ids...), true
}
