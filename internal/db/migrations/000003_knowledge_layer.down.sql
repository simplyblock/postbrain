-- Migration 000003 rollback: drop in reverse order.

-- Remove pg_cron job
SELECT cron.unschedule('detect-stale-knowledge-age');

-- Remove forward FK added to memories
ALTER TABLE {{POSTBRAIN_SCHEMA}}.memories DROP CONSTRAINT IF EXISTS memories_promoted_to_fk;

-- Triggers
DROP TRIGGER IF EXISTS knowledge_artifacts_updated_at ON {{POSTBRAIN_SCHEMA}}.knowledge_artifacts;
DROP TRIGGER IF EXISTS knowledge_collections_updated_at ON {{POSTBRAIN_SCHEMA}}.knowledge_collections;

-- Tables (reverse creation order)
DROP TABLE IF EXISTS {{POSTBRAIN_SCHEMA}}.consolidations CASCADE;
DROP TABLE IF EXISTS {{POSTBRAIN_SCHEMA}}.staleness_flags CASCADE;
DROP TABLE IF EXISTS {{POSTBRAIN_SCHEMA}}.promotion_requests CASCADE;
DROP TABLE IF EXISTS {{POSTBRAIN_SCHEMA}}.sharing_grants CASCADE;
DROP TABLE IF EXISTS {{POSTBRAIN_SCHEMA}}.knowledge_collection_items CASCADE;
DROP TABLE IF EXISTS {{POSTBRAIN_SCHEMA}}.knowledge_collections CASCADE;
DROP TABLE IF EXISTS {{POSTBRAIN_SCHEMA}}.knowledge_history CASCADE;
DROP TABLE IF EXISTS {{POSTBRAIN_SCHEMA}}.knowledge_endorsements CASCADE;
DROP TABLE IF EXISTS {{POSTBRAIN_SCHEMA}}.knowledge_artifacts CASCADE;
