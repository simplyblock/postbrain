-- Migration 000004 rollback.

-- Trigger on events (must be dropped before function)
DROP TRIGGER IF EXISTS events_skill_stats ON events;
DROP FUNCTION IF EXISTS skills_update_invocation_stats();

-- Trigger on skills
DROP TRIGGER IF EXISTS skills_updated_at ON skills;

-- Tables (reverse creation order)
DROP TABLE IF EXISTS skill_history CASCADE;
DROP TABLE IF EXISTS skill_endorsements CASCADE;
DROP TABLE IF EXISTS skills CASCADE;
