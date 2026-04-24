ALTER TABLE {{POSTBRAIN_SCHEMA}}.memories
    ADD COLUMN IF NOT EXISTS parent_memory_id UUID REFERENCES {{POSTBRAIN_SCHEMA}}.memories(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS memories_parent_id_idx
    ON {{POSTBRAIN_SCHEMA}}.memories (parent_memory_id)
    WHERE parent_memory_id IS NOT NULL;
