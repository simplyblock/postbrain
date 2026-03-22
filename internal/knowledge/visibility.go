package knowledge

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
)

// ResolveVisibleScopeIDs returns the scope IDs from which the caller can read
// published knowledge artifacts. It walks the ltree ancestor chain from
// requestedScopeID and combines with the principal's personal scope.
//
// Company-wide artifacts (visibility='company') are always included in queries
// via the visibility filter in the DB layer, not here.
func ResolveVisibleScopeIDs(ctx context.Context, pool *pgxpool.Pool, principalID uuid.UUID, requestedScopeID uuid.UUID) ([]uuid.UUID, error) {
	// 1. Get ancestor scope IDs via ltree (includes requestedScopeID itself).
	ancestors, err := db.GetAncestorScopeIDs(ctx, pool, requestedScopeID)
	if err != nil {
		return nil, fmt.Errorf("knowledge: resolve visible scopes: %w", err)
	}

	// 2. Get the principal's personal scope (if any).
	// Personal scopes have kind='personal' and principal_id = principalID.
	personalScope, err := getPersonalScope(ctx, pool, principalID)
	if err != nil {
		return nil, fmt.Errorf("knowledge: resolve visible scopes personal: %w", err)
	}

	combined := make([]uuid.UUID, 0, len(ancestors)+1)
	combined = append(combined, ancestors...)
	if personalScope != nil {
		combined = append(combined, personalScope.ID)
	}

	return deduplicateScopeIDs(combined), nil
}

// getPersonalScope returns the personal scope for a principal, or nil if none exists.
func getPersonalScope(ctx context.Context, pool *pgxpool.Pool, principalID uuid.UUID) (*db.Scope, error) {
	scope, err := db.GetScopeByExternalID(ctx, pool, "personal", principalID.String())
	if err != nil {
		return nil, err
	}
	return scope, nil
}

// deduplicateScopeIDs returns a new slice with duplicate UUIDs removed, preserving order.
func deduplicateScopeIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	result := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}
	return result
}
