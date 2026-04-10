-- Additive indexes for cross-scope time-window recall performance.

CREATE INDEX IF NOT EXISTS memories_scope_active_created_at_idx
    ON memories (scope_id, is_active, created_at DESC);

CREATE INDEX IF NOT EXISTS knowledge_owner_status_published_at_idx
    ON knowledge_artifacts (owner_scope_id, status, published_at DESC);

CREATE INDEX IF NOT EXISTS knowledge_owner_status_created_at_idx
    ON knowledge_artifacts (owner_scope_id, status, created_at DESC);
