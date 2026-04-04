-- Ensure deleting a scope also removes promotion requests targeting that scope.
ALTER TABLE promotion_requests
    DROP CONSTRAINT IF EXISTS promotion_requests_target_scope_id_fkey;

ALTER TABLE promotion_requests
    ADD CONSTRAINT promotion_requests_target_scope_id_fkey
    FOREIGN KEY (target_scope_id) REFERENCES scopes(id) ON DELETE CASCADE;

