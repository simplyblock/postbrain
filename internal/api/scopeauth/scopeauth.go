package scopeauth

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
)

var (
	// ErrTokenScopeDenied indicates the requested scope is outside token scope_ids.
	ErrTokenScopeDenied = errors.New("scopeauth: token scope denied")
	// ErrPrincipalScopeDenied indicates the requested scope is outside principal effective scopes.
	ErrPrincipalScopeDenied = errors.New("scopeauth: principal scope denied")
)

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
