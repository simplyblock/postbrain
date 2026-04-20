DROP INDEX IF EXISTS {{POSTBRAIN_SCHEMA}}.knowledge_artifacts_artifact_kind_idx;
DROP INDEX IF EXISTS {{POSTBRAIN_SCHEMA}}.knowledge_artifacts_supersedes_idx;

ALTER TABLE {{POSTBRAIN_SCHEMA}}.knowledge_artifacts
    DROP CONSTRAINT IF EXISTS knowledge_artifacts_artifact_kind_check;

ALTER TABLE {{POSTBRAIN_SCHEMA}}.knowledge_artifacts
    DROP COLUMN IF EXISTS supersedes_artifact_id;

ALTER TABLE {{POSTBRAIN_SCHEMA}}.knowledge_artifacts
    DROP COLUMN IF EXISTS occurred_at;

ALTER TABLE {{POSTBRAIN_SCHEMA}}.knowledge_artifacts
    DROP COLUMN IF EXISTS artifact_meta;

ALTER TABLE {{POSTBRAIN_SCHEMA}}.knowledge_artifacts
    DROP COLUMN IF EXISTS artifact_kind;
