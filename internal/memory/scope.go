// Package memory provides memory creation, recall, consolidation, and
// scope fan-out for the Postbrain memory layer.
package memory

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
)

// FanOutScopeIDs returns all scope IDs visible from the given starting scope:
//   - All ancestor scopes (using ltree @>)
//   - The personal scope of the principal
//   - Optionally capped by maxDepth (0 = no limit)
//
// If strictScope is true: returns only [scopeID].
func FanOutScopeIDs(ctx context.Context, pool *pgxpool.Pool, scopeID, principalID uuid.UUID, maxDepth int, strictScope bool) ([]uuid.UUID, error) {
	if strictScope {
		return []uuid.UUID{scopeID}, nil
	}

	ancestors, err := db.GetAncestorScopeIDs(ctx, pool, scopeID)
	if err != nil {
		return nil, fmt.Errorf("memory: fan-out ancestors: %w", err)
	}

	// Apply maxDepth: filter out scopes beyond the depth limit.
	// We do this by checking each scope's path depth from the DB.
	if maxDepth > 0 && len(ancestors) > 0 {
		ancestors, err = filterByDepth(ctx, pool, ancestors, maxDepth)
		if err != nil {
			return nil, fmt.Errorf("memory: fan-out depth filter: %w", err)
		}
	}

	personal, err := personalScopeIDs(ctx, pool, principalID)
	if err != nil {
		return nil, fmt.Errorf("memory: fan-out personal scope: %w", err)
	}

	return deduplicateScopeIDs(append(ancestors, personal...)), nil
}

// filterByDepth returns only scope IDs whose ltree path depth <= maxDepth.
func filterByDepth(ctx context.Context, pool *pgxpool.Pool, ids []uuid.UUID, maxDepth int) ([]uuid.UUID, error) {
	rows, err := pool.Query(ctx,
		`SELECT id FROM scopes WHERE id = ANY($1) AND nlevel(path) <= $2`,
		ids, maxDepth,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var filtered []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		filtered = append(filtered, id)
	}
	return filtered, rows.Err()
}

// personalScopeIDs returns the scope IDs where kind='user' AND principal_id = principalID.
func personalScopeIDs(ctx context.Context, pool *pgxpool.Pool, principalID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := pool.Query(ctx,
		`SELECT id FROM scopes WHERE kind='user' AND principal_id = $1`,
		principalID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// deduplicateScopeIDs returns a slice with all duplicate UUIDs removed,
// preserving order.
func deduplicateScopeIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}

// fanOutStrict is an internal helper exposed to tests: returns [scopeID].
func fanOutStrict(scopeID, _ uuid.UUID) ([]uuid.UUID, error) {
	return []uuid.UUID{scopeID}, nil
}
