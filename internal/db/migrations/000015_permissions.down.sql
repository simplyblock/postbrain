-- Migration 000015 rollback: permissions system

DROP TABLE IF EXISTS {{POSTBRAIN_SCHEMA}}.scope_grants;

ALTER TABLE {{POSTBRAIN_SCHEMA}}.principals
    DROP COLUMN IF EXISTS is_system_admin;
