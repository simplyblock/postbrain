-- Migration 000001 rollback: drop all objects in reverse creation order.

-- Events (partitioned) — partman config is removed by CASCADE when table is dropped
DROP TABLE IF EXISTS events CASCADE;

-- Sessions
DROP TABLE IF EXISTS sessions CASCADE;

-- Scopes
DROP TRIGGER IF EXISTS scopes_path_trigger ON scopes;
DROP FUNCTION IF EXISTS scopes_compute_path();
DROP TABLE IF EXISTS scopes CASCADE;

-- Principal memberships
DROP TABLE IF EXISTS principal_memberships CASCADE;

-- Tokens
DROP TABLE IF EXISTS tokens CASCADE;

-- Principals
DROP TRIGGER IF EXISTS principals_updated_at ON principals;
DROP TABLE IF EXISTS principals CASCADE;

-- touch_updated_at function
DROP FUNCTION IF EXISTS touch_updated_at();

-- Embedding models
DROP TABLE IF EXISTS embedding_models CASCADE;

-- FTS configuration
DROP TEXT SEARCH CONFIGURATION IF EXISTS postbrain_fts;

-- Extensions (in reverse order)
DROP EXTENSION IF EXISTS pg_partman;
DROP EXTENSION IF EXISTS pg_cron;
DROP EXTENSION IF EXISTS pg_prewarm;
DROP EXTENSION IF EXISTS fuzzystrmatch;
DROP EXTENSION IF EXISTS unaccent;
DROP EXTENSION IF EXISTS citext;
DROP EXTENSION IF EXISTS ltree;
DROP EXTENSION IF EXISTS btree_gin;
DROP EXTENSION IF EXISTS pg_trgm;
DROP EXTENSION IF EXISTS vector;
