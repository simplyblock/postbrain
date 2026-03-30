-- Repository attachment for project-kind scopes.
ALTER TABLE scopes
    ADD COLUMN IF NOT EXISTS repo_url             TEXT,
    ADD COLUMN IF NOT EXISTS repo_default_branch  TEXT NOT NULL DEFAULT 'main',
    ADD COLUMN IF NOT EXISTS last_indexed_commit   TEXT;

-- source_file tracking on relations so incremental re-index can invalidate
-- stale edges before inserting the fresh set for a changed file.
ALTER TABLE relations
    ADD COLUMN IF NOT EXISTS source_file TEXT;

CREATE INDEX IF NOT EXISTS relations_source_file_idx
    ON relations (scope_id, source_file)
    WHERE source_file IS NOT NULL;
