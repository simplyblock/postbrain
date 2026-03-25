-- Migration 000007: artifact entity graph
-- Adds artifact_entities join table and source_artifact traceability on relations.

-- ── Artifact ↔ Entity links ────────────────────────────────
CREATE TABLE artifact_entities (
    artifact_id UUID NOT NULL REFERENCES knowledge_artifacts(id) ON DELETE CASCADE,
    entity_id   UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    role        TEXT CHECK (role IN ('subject', 'object', 'context', 'related')),
    PRIMARY KEY (artifact_id, entity_id)
);

CREATE INDEX artifact_entities_entity_idx ON artifact_entities (entity_id);

-- ── Trace relations back to their source artifact ──────────
ALTER TABLE relations
    ADD COLUMN source_artifact UUID REFERENCES knowledge_artifacts(id) ON DELETE SET NULL;
