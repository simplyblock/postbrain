-- Migration 000015 rollback: permissions system

DROP TABLE IF EXISTS scope_grants;

ALTER TABLE principals
    DROP COLUMN IF EXISTS is_system_admin;
