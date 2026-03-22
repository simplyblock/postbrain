-- Migration 000005: Apache AGE graph overlay (optional).
-- This migration is a no-op if AGE is not installed.
-- It wraps all AGE operations in an exception handler so the migration
-- succeeds cleanly even when the extension is absent.

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
