-- Migration 000004 rollback.

-- Trigger on events (must be dropped before function)
DROP TRIGGER IF EXISTS events_skill_stats ON {{POSTBRAIN_SCHEMA}}.events;
DROP FUNCTION IF EXISTS {{POSTBRAIN_SCHEMA}}.skills_update_invocation_stats();

-- Trigger on skills
DROP TRIGGER IF EXISTS skills_updated_at ON {{POSTBRAIN_SCHEMA}}.skills;

-- Tables (reverse creation order)
DROP TABLE IF EXISTS {{POSTBRAIN_SCHEMA}}.skill_history CASCADE;
DROP TABLE IF EXISTS {{POSTBRAIN_SCHEMA}}.skill_endorsements CASCADE;
DROP TABLE IF EXISTS {{POSTBRAIN_SCHEMA}}.skills CASCADE;
