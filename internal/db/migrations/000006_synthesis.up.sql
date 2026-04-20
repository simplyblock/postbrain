-- Migration 000006: topic synthesis & source suppression
-- Adds digest knowledge_type, artifact_digest_sources join table,
-- and knowledge_digest_log audit table.

-- ─────────────────────────────────────────
-- 1. Extend knowledge_type to include 'digest'
-- ─────────────────────────────────────────
ALTER TABLE {{POSTBRAIN_SCHEMA}}.knowledge_artifacts
    DROP CONSTRAINT knowledge_artifacts_knowledge_type_check;
ALTER TABLE {{POSTBRAIN_SCHEMA}}.knowledge_artifacts
    ADD CONSTRAINT knowledge_artifacts_knowledge_type_check
    CHECK (knowledge_type IN ('semantic', 'episodic', 'procedural', 'reference', 'digest'));

-- ─────────────────────────────────────────
-- 2. Digest → source join table
-- ─────────────────────────────────────────
CREATE TABLE {{POSTBRAIN_SCHEMA}}.artifact_digest_sources (
    digest_id   UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.knowledge_artifacts(id) ON DELETE CASCADE,
    source_id   UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.knowledge_artifacts(id) ON DELETE CASCADE,
    added_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (digest_id, source_id)
);

-- Reverse index: which digests cover this source?
CREATE INDEX artifact_digest_sources_source_idx
    ON {{POSTBRAIN_SCHEMA}}.artifact_digest_sources (source_id);

-- ─────────────────────────────────────────
-- 3. Synthesis audit log
-- ─────────────────────────────────────────
CREATE TABLE {{POSTBRAIN_SCHEMA}}.knowledge_digest_log (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    scope_id        UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.scopes(id) ON DELETE CASCADE,
    digest_id       UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.knowledge_artifacts(id) ON DELETE CASCADE,
    source_ids      UUID[] NOT NULL,
    strategy        TEXT NOT NULL CHECK (strategy IN ('manual', 'auto_cluster')),
    synthesised_by  UUID REFERENCES {{POSTBRAIN_SCHEMA}}.principals(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX knowledge_digest_log_digest_idx ON {{POSTBRAIN_SCHEMA}}.knowledge_digest_log (digest_id);
CREATE INDEX knowledge_digest_log_scope_idx  ON {{POSTBRAIN_SCHEMA}}.knowledge_digest_log (scope_id, created_at DESC);
