-- Migration 000016 rollback: token permissions v2

ALTER TABLE tokens
    DROP CONSTRAINT IF EXISTS tokens_no_admin_permission;
