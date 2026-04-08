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

const ensureAGEPrivilegesSQL = `
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname='age') THEN
        BEGIN
            GRANT USAGE ON SCHEMA ag_catalog TO PUBLIC;
        EXCEPTION WHEN insufficient_privilege THEN
            RAISE NOTICE 'insufficient privilege to grant USAGE on ag_catalog; continuing with runtime probe';
        END;
        BEGIN
            GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA ag_catalog TO PUBLIC;
        EXCEPTION WHEN insufficient_privilege THEN
            RAISE NOTICE 'insufficient privilege to grant EXECUTE on ag_catalog functions; continuing with runtime probe';
        END;
        BEGIN
            GRANT USAGE ON TYPE ag_catalog.agtype TO PUBLIC;
        EXCEPTION WHEN insufficient_privilege THEN
            RAISE NOTICE 'insufficient privilege to grant USAGE on ag_catalog.agtype; continuing with runtime probe';
        END;
        BEGIN
            IF EXISTS (SELECT 1 FROM pg_namespace WHERE nspname='postbrain') THEN
                GRANT USAGE ON SCHEMA postbrain TO PUBLIC;
                GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA postbrain TO PUBLIC;
                GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA postbrain TO PUBLIC;
            END IF;
        EXCEPTION WHEN insufficient_privilege THEN
            RAISE NOTICE 'insufficient privilege to grant USAGE on postbrain schema; continuing with runtime probe';
        END;
    END IF;
END;
$$;
`

const ensureAGEAccessProbeSQL = `
SELECT * FROM ag_catalog.cypher('postbrain', $$
CREATE (n:Entity)
SET n.id = '__postbrain_age_startup_probe__'
WITH n
DELETE n
RETURN 1
$$) AS (result ag_catalog.agtype);
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
	if _, err := pool.Exec(ctx, ensureAGEPrivilegesSQL); err != nil {
		return fmt.Errorf("ensure age privileges: %w", err)
	}
	var ageInstalled bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname='age')").Scan(&ageInstalled); err != nil {
		return fmt.Errorf("ensure age overlay: detect extension: %w", err)
	}
	if ageInstalled {
		var graphSchemaExists bool
		if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname='postbrain')").Scan(&graphSchemaExists); err != nil {
			return fmt.Errorf("ensure age overlay: detect graph schema: %w", err)
		}
		if graphSchemaExists {
			var hasUsage bool
			if err := pool.QueryRow(ctx, "SELECT has_schema_privilege(current_user, 'postbrain', 'USAGE')").Scan(&hasUsage); err != nil {
				return fmt.Errorf("ensure age overlay: check postbrain schema usage: %w", err)
			}
			if !hasUsage {
				return fmt.Errorf("ensure age overlay: role %q lacks USAGE on schema postbrain", pool.Config().ConnConfig.User)
			}
		}
		if _, err := pool.Exec(ctx, ensureAGEAccessProbeSQL); err != nil {
			return fmt.Errorf("ensure age access probe: %w", err)
		}
	}
	return nil
}
