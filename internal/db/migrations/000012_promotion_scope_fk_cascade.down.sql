-- Revert promotion target scope FK behavior to non-cascading deletes.
ALTER TABLE {{POSTBRAIN_SCHEMA}}.promotion_requests
    DROP CONSTRAINT IF EXISTS promotion_requests_target_scope_id_fkey;

ALTER TABLE {{POSTBRAIN_SCHEMA}}.promotion_requests
    ADD CONSTRAINT promotion_requests_target_scope_id_fkey
    FOREIGN KEY (target_scope_id) REFERENCES {{POSTBRAIN_SCHEMA}}.scopes(id);

