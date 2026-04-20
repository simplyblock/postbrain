-- Migration 000013: embedding index
-- Extends embedding_models with provider/runtime metadata,
-- introduces the central embedding_index tracking table,
-- while keeping legacy embedding_model_id FK columns for compatibility.

-- ─────────────────────────────────────────
-- 1. Extend embedding_models with provider/runtime columns
-- ─────────────────────────────────────────
ALTER TABLE {{POSTBRAIN_SCHEMA}}.embedding_models
    ADD COLUMN provider       TEXT,
    ADD COLUMN service_url    TEXT,
    ADD COLUMN provider_model TEXT,
    ADD COLUMN table_name     TEXT,
    ADD COLUMN is_ready       BOOLEAN NOT NULL DEFAULT false;

-- ─────────────────────────────────────────
-- 2. Central embedding index table
-- Tracks which objects have embeddings in which per-model vector tables,
-- including backfill/re-embed status and retry state.
-- ─────────────────────────────────────────
CREATE TABLE {{POSTBRAIN_SCHEMA}}.embedding_index (
    object_type  TEXT        NOT NULL,
    object_id    UUID        NOT NULL,
    model_id     UUID        NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.embedding_models(id),
    status       TEXT        NOT NULL DEFAULT 'pending',
    retry_count  INT         NOT NULL DEFAULT 0,
    last_error   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY  (object_type, object_id, model_id),
    CONSTRAINT embedding_index_object_type_check
        CHECK (object_type IN ('memory', 'entity', 'knowledge_artifact', 'skill')),
    CONSTRAINT embedding_index_status_check
        CHECK (status IN ('pending', 'ready', 'failed'))
);

CREATE INDEX embedding_index_model_status_idx
    ON {{POSTBRAIN_SCHEMA}}.embedding_index (model_id, status);

CREATE TRIGGER embedding_index_updated_at BEFORE UPDATE ON {{POSTBRAIN_SCHEMA}}.embedding_index
    FOR EACH ROW EXECUTE FUNCTION {{POSTBRAIN_SCHEMA}}.touch_updated_at();

-- ─────────────────────────────────────────
-- 3. Compatibility note
-- Legacy embedding_model_id columns on object tables are intentionally retained
-- in this migration. Cleanup is deferred to a dedicated later migration after
-- dual-read/dual-write cutover is complete.
-- ─────────────────────────────────────────
