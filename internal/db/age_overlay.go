package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

const ensureAGEOverlaySQL = `
DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS age;
    LOAD 'age';
    SET search_path = ag_catalog, "$user", public;
    PERFORM create_graph('postbrain');
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'Apache AGE not available, skipping graph setup: %', SQLERRM;
END;
$$;
`

// EnsureAGEOverlay is idempotent and best-effort.
//
// It allows enabling AGE later even when the original AGE migration ran while
// the extension was unavailable.
func EnsureAGEOverlay(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return fmt.Errorf("ensure age overlay: nil pool")
	}
	if _, err := pool.Exec(ctx, ensureAGEOverlaySQL); err != nil {
		return fmt.Errorf("ensure age overlay: %w", err)
	}
	return nil
}
