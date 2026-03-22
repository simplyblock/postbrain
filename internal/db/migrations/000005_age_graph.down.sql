-- Migration 000005 rollback: drop AGE graph objects if present.
-- Wrapped in an exception handler so it is a no-op when AGE is absent.

DO $$
BEGIN
    LOAD 'age';
    SET search_path = ag_catalog, "$user", public;
    PERFORM drop_graph('postbrain', true);
    DROP EXTENSION IF EXISTS age;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'Apache AGE not available or graph already absent, skipping: %', SQLERRM;
END;
$$;
