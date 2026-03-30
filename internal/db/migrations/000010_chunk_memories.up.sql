ALTER TABLE memories
    ADD COLUMN IF NOT EXISTS parent_memory_id UUID REFERENCES memories(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS memories_parent_id_idx
    ON memories (parent_memory_id)
    WHERE parent_memory_id IS NOT NULL;
