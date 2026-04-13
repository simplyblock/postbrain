package jobs

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
)

// ExpireWorkingMemory soft-deletes all active memories whose expires_at < now().
// This supplements the pg_cron job (run on-demand or at startup for any expired items
// that accumulated while the server was down).
// Returns the number of rows updated.
func ExpireWorkingMemory(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	return db.New(pool).ExpireWorkingMemories(ctx)
}
