package jobs

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ExpireWorkingMemory soft-deletes all active memories whose expires_at < now().
// This supplements the pg_cron job (run on-demand or at startup for any expired items
// that accumulated while the server was down).
// Returns the number of rows updated.
func ExpireWorkingMemory(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx,
		`UPDATE memories SET is_active = false
		 WHERE expires_at < now() AND is_active = true`,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
