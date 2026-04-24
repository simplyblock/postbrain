-- Migration 000002: memory graph
-- Creates memories, entities, memory_entities, relations tables,
-- touch_updated_at triggers, and pg_cron housekeeping jobs.

-- ─────────────────────────────────────────
-- 1. Memories table + indexes
-- ─────────────────────────────────────────
CREATE TABLE {{POSTBRAIN_SCHEMA}}.memories (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),

    memory_type     TEXT NOT NULL CHECK (memory_type IN ('semantic', 'episodic', 'procedural', 'working')),
    scope_id        UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.scopes(id) ON DELETE CASCADE,
    author_id       UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.principals(id),

    content              TEXT NOT NULL,
    summary              TEXT,
    embedding            vector(1536),
    embedding_model_id   UUID REFERENCES {{POSTBRAIN_SCHEMA}}.embedding_models(id),
    embedding_code       vector(1024),
    embedding_code_model_id UUID REFERENCES {{POSTBRAIN_SCHEMA}}.embedding_models(id),
    content_kind         TEXT NOT NULL DEFAULT 'text' CHECK (content_kind IN ('text', 'code')),
    meta                 JSONB NOT NULL DEFAULT '{}',

    version         INT NOT NULL DEFAULT 1,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    confidence      FLOAT NOT NULL DEFAULT 1.0 CHECK (confidence BETWEEN 0 AND 1),
    importance      FLOAT NOT NULL DEFAULT 0.5 CHECK (importance BETWEEN 0 AND 1),
    access_count    INT NOT NULL DEFAULT 0,
    last_accessed   TIMESTAMPTZ,

    expires_at      TIMESTAMPTZ,

    promotion_status  TEXT CHECK (promotion_status IN ('none', 'nominated', 'promoted'))
                      NOT NULL DEFAULT 'none',
    promoted_to       UUID,

    source_ref      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Text embedding ANN index
CREATE INDEX memories_embedding_hnsw_idx
    ON {{POSTBRAIN_SCHEMA}}.memories USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- Code embedding ANN index (partial — only code memories)
CREATE INDEX memories_embedding_code_hnsw_idx
    ON {{POSTBRAIN_SCHEMA}}.memories USING hnsw (embedding_code vector_cosine_ops)
    WITH (m = 16, ef_construction = 64)
    WHERE embedding_code IS NOT NULL;

-- Full-text search index
CREATE INDEX memories_content_fts_idx
    ON {{POSTBRAIN_SCHEMA}}.memories USING GIN (to_tsvector('postbrain_fts', content));

-- Trigram index for partial/fuzzy keyword search
CREATE INDEX memories_content_trgm_idx
    ON {{POSTBRAIN_SCHEMA}}.memories USING GIN (content gin_trgm_ops);

-- Composite index for scope-filtered queries
CREATE INDEX memories_scope_type_idx
    ON {{POSTBRAIN_SCHEMA}}.memories (scope_id, memory_type, is_active);

-- TTL expiry index for working memory cleanup
CREATE INDEX memories_expires_at_idx
    ON {{POSTBRAIN_SCHEMA}}.memories (expires_at)
    WHERE expires_at IS NOT NULL;

-- ─────────────────────────────────────────
-- 2. Entities table + HNSW index
-- ─────────────────────────────────────────
CREATE TABLE {{POSTBRAIN_SCHEMA}}.entities (
    id                 UUID PRIMARY KEY DEFAULT uuidv7(),
    scope_id           UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.scopes(id) ON DELETE CASCADE,
    entity_type        TEXT NOT NULL,
    name               TEXT NOT NULL,
    canonical          TEXT NOT NULL,
    meta               JSONB NOT NULL DEFAULT '{}',
    embedding          vector(1536),
    embedding_model_id UUID REFERENCES {{POSTBRAIN_SCHEMA}}.embedding_models(id),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (scope_id, entity_type, canonical)
);

CREATE INDEX entities_embedding_hnsw_idx
    ON {{POSTBRAIN_SCHEMA}}.entities USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- ─────────────────────────────────────────
-- 3. Memory ↔ Entity links
-- ─────────────────────────────────────────
CREATE TABLE {{POSTBRAIN_SCHEMA}}.memory_entities (
    memory_id   UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.memories(id) ON DELETE CASCADE,
    entity_id   UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.entities(id) ON DELETE CASCADE,
    role        TEXT CHECK (role IN ('subject', 'object', 'context', 'related')),
    PRIMARY KEY (memory_id, entity_id)
);

-- ─────────────────────────────────────────
-- 4. Entity ↔ Entity relations
-- ─────────────────────────────────────────
CREATE TABLE {{POSTBRAIN_SCHEMA}}.relations (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    scope_id        UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.scopes(id) ON DELETE CASCADE,
    subject_id      UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.entities(id) ON DELETE CASCADE,
    predicate       TEXT NOT NULL,
    object_id       UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.entities(id) ON DELETE CASCADE,
    confidence      FLOAT NOT NULL DEFAULT 1.0,
    source_memory   UUID REFERENCES {{POSTBRAIN_SCHEMA}}.memories(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (scope_id, subject_id, predicate, object_id)
);

CREATE INDEX relations_subject_idx ON {{POSTBRAIN_SCHEMA}}.relations (subject_id, predicate);
CREATE INDEX relations_object_idx  ON {{POSTBRAIN_SCHEMA}}.relations (object_id, predicate);

-- ─────────────────────────────────────────
-- 5. touch_updated_at triggers
-- ─────────────────────────────────────────
CREATE TRIGGER memories_updated_at BEFORE UPDATE ON {{POSTBRAIN_SCHEMA}}.memories
    FOR EACH ROW EXECUTE FUNCTION {{POSTBRAIN_SCHEMA}}.touch_updated_at();

CREATE TRIGGER entities_updated_at BEFORE UPDATE ON {{POSTBRAIN_SCHEMA}}.entities
    FOR EACH ROW EXECUTE FUNCTION {{POSTBRAIN_SCHEMA}}.touch_updated_at();

-- ─────────────────────────────────────────
-- 6. pg_cron housekeeping jobs
-- ─────────────────────────────────────────

-- Every 5 min: expire TTL-based working memories
SELECT cron.schedule('expire-working-memory', '*/5 * * * *', $$
    UPDATE memories
    SET    is_active = false
    WHERE  expires_at < now()
    AND    is_active = true
$$);

-- Nightly at 03:00: decay importance scores
SELECT cron.schedule('decay-memory-importance', '0 3 * * *', $$
    UPDATE memories
    SET    importance = GREATEST(0.0,
               importance * exp(
                   -CASE memory_type
                       WHEN 'working'    THEN 0.015
                       WHEN 'episodic'   THEN 0.010
                       ELSE                   0.005
                    END
                   * GREATEST(0, EXTRACT(EPOCH FROM
                       (now() - COALESCE(last_accessed, created_at))
                     ) / 86400.0)
               )
           )
    WHERE  is_active = true
$$);

-- Weekly on Sunday at 04:00: soft-delete low-value decayed memories
SELECT cron.schedule('prune-low-value-memories', '0 4 * * 0', $$
    UPDATE memories
    SET    is_active = false
    WHERE  is_active = true
    AND    importance < 0.05
    AND    access_count < 2
    AND    memory_type IN ('episodic', 'working')
$$);
