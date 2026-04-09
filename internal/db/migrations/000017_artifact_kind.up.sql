ALTER TABLE knowledge_artifacts
    ADD COLUMN artifact_kind TEXT NOT NULL DEFAULT 'general';

ALTER TABLE knowledge_artifacts
    ADD COLUMN artifact_meta JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE knowledge_artifacts
    ADD COLUMN occurred_at TIMESTAMPTZ;

ALTER TABLE knowledge_artifacts
    ADD COLUMN supersedes_artifact_id UUID REFERENCES knowledge_artifacts(id) ON DELETE SET NULL;

ALTER TABLE knowledge_artifacts
    ADD CONSTRAINT knowledge_artifacts_artifact_kind_check
    CHECK (artifact_kind IN ('general', 'decision', 'meeting_note', 'retrospective', 'spec', 'design_doc', 'research'));

CREATE INDEX knowledge_artifacts_artifact_kind_idx
    ON knowledge_artifacts (artifact_kind, status, owner_scope_id);

CREATE INDEX knowledge_artifacts_supersedes_idx
    ON knowledge_artifacts (supersedes_artifact_id)
    WHERE supersedes_artifact_id IS NOT NULL;
