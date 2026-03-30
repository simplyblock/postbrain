DROP INDEX IF EXISTS relations_source_file_idx;
ALTER TABLE relations DROP COLUMN IF EXISTS source_file;
ALTER TABLE scopes DROP COLUMN IF EXISTS last_indexed_commit;
ALTER TABLE scopes DROP COLUMN IF EXISTS repo_default_branch;
ALTER TABLE scopes DROP COLUMN IF EXISTS repo_url;
