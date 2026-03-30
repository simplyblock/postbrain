DROP INDEX IF EXISTS memories_parent_id_idx;
ALTER TABLE memories DROP COLUMN IF EXISTS parent_memory_id;
