package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

const ensureAGEPrivilegesSQL = `
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname='age') THEN
        BEGIN
            EXECUTE 'GRANT USAGE ON SCHEMA ag_catalog TO ' || quote_ident(current_user);
        EXCEPTION WHEN insufficient_privilege THEN
            RAISE NOTICE 'insufficient privilege to grant USAGE on ag_catalog; continuing with runtime probe';
        END;
        BEGIN
            EXECUTE 'GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA ag_catalog TO ' || quote_ident(current_user);
        EXCEPTION WHEN insufficient_privilege THEN
            RAISE NOTICE 'insufficient privilege to grant EXECUTE on ag_catalog functions; continuing with runtime probe';
        END;
        BEGIN
            EXECUTE 'GRANT USAGE ON TYPE ag_catalog.agtype TO ' || quote_ident(current_user);
        EXCEPTION WHEN insufficient_privilege THEN
            RAISE NOTICE 'insufficient privilege to grant USAGE on ag_catalog.agtype; continuing with runtime probe';
        END;
        BEGIN
            IF EXISTS (SELECT 1 FROM pg_namespace WHERE nspname='postbrain') THEN
                EXECUTE 'GRANT USAGE ON SCHEMA postbrain TO ' || quote_ident(current_user);
                EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA postbrain TO ' || quote_ident(current_user);
                EXECUTE 'GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA postbrain TO ' || quote_ident(current_user);
            END IF;
        EXCEPTION WHEN insufficient_privilege THEN
            RAISE NOTICE 'insufficient privilege to grant USAGE on postbrain schema; continuing with runtime probe';
        END;
    END IF;
END;
$$;
`

const ensureAGEAccessProbeSQL = `
SELECT * FROM ag_catalog.cypher('postbrain', $$ RETURN 1 $$) AS (result ag_catalog.agtype);
`

type ageOverlayExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// EnsureAGEOverlay is idempotent and best-effort.
//
// It allows enabling AGE later even when the original AGE migration ran while
// the extension was unavailable.
func EnsureAGEOverlay(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return fmt.Errorf("ensure age overlay: nil pool")
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("ensure age overlay: acquire conn: %w", err)
	}
	defer conn.Release()

	return ensureAGEOverlayOnExecutor(ctx, conn, pool.Config().ConnConfig.User)
}

func ensureAGEOverlayOnExecutor(ctx context.Context, exec ageOverlayExecutor, currentUser string) error {
	if _, err := exec.Exec(ctx, ensureAGEOverlaySQL); err != nil {
		return fmt.Errorf("ensure age overlay: %w", err)
	}
	if _, err := exec.Exec(ctx, ensureAGEPrivilegesSQL); err != nil {
		return fmt.Errorf("ensure age privileges: %w", err)
	}
	var ageInstalled bool
	if err := exec.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname='age')").Scan(&ageInstalled); err != nil {
		return fmt.Errorf("ensure age overlay: detect extension: %w", err)
	}
	if ageInstalled {
		var graphSchemaExists bool
		if err := exec.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname='postbrain')").Scan(&graphSchemaExists); err != nil {
			return fmt.Errorf("ensure age overlay: detect graph schema: %w", err)
		}
		if graphSchemaExists {
			var hasUsage bool
			if err := exec.QueryRow(ctx, "SELECT has_schema_privilege(current_user, 'postbrain', 'USAGE')").Scan(&hasUsage); err != nil {
				return fmt.Errorf("ensure age overlay: check postbrain schema usage: %w", err)
			}
			if !hasUsage {
				return fmt.Errorf("ensure age overlay: role %q lacks USAGE on schema postbrain", currentUser)
			}
		}
		if _, err := exec.Exec(ctx, ensureAGEAccessProbeSQL); err != nil {
			return fmt.Errorf("ensure age access probe: %w", err)
		}
	}
	return nil
}
