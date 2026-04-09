DROP INDEX IF EXISTS knowledge_artifacts_artifact_kind_idx;
DROP INDEX IF EXISTS knowledge_artifacts_supersedes_idx;

ALTER TABLE knowledge_artifacts
    DROP CONSTRAINT IF EXISTS knowledge_artifacts_artifact_kind_check;

ALTER TABLE knowledge_artifacts
    DROP COLUMN IF EXISTS supersedes_artifact_id;

ALTER TABLE knowledge_artifacts
    DROP COLUMN IF EXISTS occurred_at;

ALTER TABLE knowledge_artifacts
    DROP COLUMN IF EXISTS artifact_meta;

ALTER TABLE knowledge_artifacts
    DROP COLUMN IF EXISTS artifact_kind;
