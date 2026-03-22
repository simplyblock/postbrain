-- Migration 000003 rollback: drop in reverse order.

-- Remove pg_cron job
SELECT cron.unschedule('detect-stale-knowledge-age');

-- Remove forward FK added to memories
ALTER TABLE memories DROP CONSTRAINT IF EXISTS memories_promoted_to_fk;

-- Triggers
DROP TRIGGER IF EXISTS knowledge_artifacts_updated_at ON knowledge_artifacts;
DROP TRIGGER IF EXISTS knowledge_collections_updated_at ON knowledge_collections;

-- Tables (reverse creation order)
DROP TABLE IF EXISTS consolidations CASCADE;
DROP TABLE IF EXISTS staleness_flags CASCADE;
DROP TABLE IF EXISTS promotion_requests CASCADE;
DROP TABLE IF EXISTS sharing_grants CASCADE;
DROP TABLE IF EXISTS knowledge_collection_items CASCADE;
DROP TABLE IF EXISTS knowledge_collections CASCADE;
DROP TABLE IF EXISTS knowledge_history CASCADE;
DROP TABLE IF EXISTS knowledge_endorsements CASCADE;
DROP TABLE IF EXISTS knowledge_artifacts CASCADE;
