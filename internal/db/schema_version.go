package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SchemaVersion reads the current schema version and dirty state from the
// schema_migrations table created by golang-migrate. Returns (0, false, nil)
// if no migrations have been applied yet (table does not exist or is empty).
func SchemaVersion(ctx context.Context, pool *pgxpool.Pool) (version uint, dirty bool, err error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("schema version: acquire: %w", err)
	}
	defer conn.Release()

	var v int64
	var d bool
	// Raw SQL intentional: schema_migrations is owned by golang-migrate, not postbrain's
	// domain model. Adding it to sqlc would require a model for an external tool's table.
	err = conn.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&v, &d)
	if err != nil {
		// Table might not exist or be empty — not an error, return zero.
		return 0, false, nil
	}
	return uint(v), d, nil
}
