-- Migration 000002 rollback: drop pg_cron jobs, then tables in reverse order.

-- Remove pg_cron jobs
SELECT cron.unschedule('prune-low-value-memories');
SELECT cron.unschedule('decay-memory-importance');
SELECT cron.unschedule('expire-working-memory');

-- Triggers
DROP TRIGGER IF EXISTS entities_updated_at ON entities;
DROP TRIGGER IF EXISTS memories_updated_at ON memories;

-- Tables (reverse creation order)
DROP TABLE IF EXISTS relations CASCADE;
DROP TABLE IF EXISTS memory_entities CASCADE;
DROP TABLE IF EXISTS entities CASCADE;
DROP TABLE IF EXISTS memories CASCADE;
