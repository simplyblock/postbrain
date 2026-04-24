-- Migration 000016: token permissions v2 — replace legacy 'admin' with full permission set

UPDATE {{POSTBRAIN_SCHEMA}}.tokens
SET permissions = ARRAY['read', 'write', 'edit', 'delete']
WHERE 'admin' = ANY(permissions);

ALTER TABLE {{POSTBRAIN_SCHEMA}}.tokens
    ADD CONSTRAINT tokens_no_admin_permission
    CHECK (NOT ('admin' = ANY(permissions)));
