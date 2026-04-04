package graph

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

const runPageRankSQL = `
WITH ranked AS (
    SELECT id, rank
    FROM age_pagerank('postbrain', 'Entity', 'RELATION', 0.85, 20)
)
UPDATE entities e
SET meta = jsonb_set(COALESCE(e.meta, '{}'::jsonb), '{pagerank}', to_jsonb(ranked.rank))
FROM ranked
WHERE e.id = ranked.id::uuid;
`

// RunPageRank computes graph centrality via AGE and writes it back to entities.meta.pagerank.
func RunPageRank(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return fmt.Errorf("graph: nil pool")
	}
	if !DetectAGE(ctx, pool) {
		return ErrAGEUnavailable
	}
	if _, err := pool.Exec(ctx, runPageRankSQL); err != nil {
		return fmt.Errorf("graph: run pagerank: %w", err)
	}
	return nil
}
