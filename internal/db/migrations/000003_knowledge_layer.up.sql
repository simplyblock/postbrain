-- Migration 000003: knowledge layer
-- Creates knowledge_artifacts, endorsements, history, collections,
-- collection items, sharing grants, promotion requests, staleness flags,
-- consolidations, and wires the memories.promoted_to FK.

-- ─────────────────────────────────────────
-- 1. knowledge_artifacts + indexes
-- ─────────────────────────────────────────
CREATE TABLE {{POSTBRAIN_SCHEMA}}.knowledge_artifacts (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),

    knowledge_type  TEXT NOT NULL CHECK (knowledge_type IN ('semantic', 'episodic', 'procedural', 'reference')),
    owner_scope_id  UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.scopes(id) ON DELETE CASCADE,
    author_id       UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.principals(id),

    visibility      TEXT NOT NULL DEFAULT 'team'
                    CHECK (visibility IN ('private', 'project', 'team', 'department', 'company')),

    status          TEXT NOT NULL DEFAULT 'draft'
                    CHECK (status IN ('draft', 'in_review', 'published', 'deprecated')),
    published_at    TIMESTAMPTZ,
    deprecated_at   TIMESTAMPTZ,
    review_required INT NOT NULL DEFAULT 1,

    title                TEXT NOT NULL,
    content              TEXT NOT NULL,
    summary              TEXT,
    embedding            vector(1536),
    embedding_model_id   UUID REFERENCES {{POSTBRAIN_SCHEMA}}.embedding_models(id),
    meta                 JSONB NOT NULL DEFAULT '{}',

    endorsement_count INT NOT NULL DEFAULT 0,
    access_count      INT NOT NULL DEFAULT 0,
    last_accessed     TIMESTAMPTZ,

    version         INT NOT NULL DEFAULT 1,
    previous_version UUID REFERENCES {{POSTBRAIN_SCHEMA}}.knowledge_artifacts(id),

    source_memory_id UUID REFERENCES {{POSTBRAIN_SCHEMA}}.memories(id) ON DELETE SET NULL,
    source_ref       TEXT,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX knowledge_embedding_hnsw_idx
    ON {{POSTBRAIN_SCHEMA}}.knowledge_artifacts USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

CREATE INDEX knowledge_owner_scope_idx
    ON {{POSTBRAIN_SCHEMA}}.knowledge_artifacts (owner_scope_id, visibility, status);

CREATE INDEX knowledge_content_fts_idx
    ON {{POSTBRAIN_SCHEMA}}.knowledge_artifacts USING GIN (to_tsvector('postbrain_fts', content));

CREATE INDEX knowledge_content_trgm_idx
    ON {{POSTBRAIN_SCHEMA}}.knowledge_artifacts USING GIN (content gin_trgm_ops);

-- ─────────────────────────────────────────
-- 2. Knowledge endorsements
-- ─────────────────────────────────────────
CREATE TABLE {{POSTBRAIN_SCHEMA}}.knowledge_endorsements (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    artifact_id     UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.knowledge_artifacts(id) ON DELETE CASCADE,
    endorser_id     UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.principals(id),
    note            TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (artifact_id, endorser_id)
);

-- ─────────────────────────────────────────
-- 3. Knowledge version history
-- ─────────────────────────────────────────
CREATE TABLE {{POSTBRAIN_SCHEMA}}.knowledge_history (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    artifact_id     UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.knowledge_artifacts(id) ON DELETE CASCADE,
    version         INT NOT NULL,
    content         TEXT NOT NULL,
    summary         TEXT,
    changed_by      UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.principals(id),
    change_note     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (artifact_id, version)
);

-- ─────────────────────────────────────────
-- 4. Knowledge collections + updated_at trigger
-- ─────────────────────────────────────────
CREATE TABLE {{POSTBRAIN_SCHEMA}}.knowledge_collections (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    scope_id    UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.scopes(id) ON DELETE CASCADE,
    owner_id    UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.principals(id),
    slug        citext NOT NULL,
    name        TEXT NOT NULL,
    description TEXT,
    visibility  TEXT NOT NULL DEFAULT 'team'
                CHECK (visibility IN ('private', 'project', 'team', 'department', 'company')),
    meta        JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (scope_id, slug)
);

CREATE TRIGGER knowledge_collections_updated_at BEFORE UPDATE ON {{POSTBRAIN_SCHEMA}}.knowledge_collections
    FOR EACH ROW EXECUTE FUNCTION {{POSTBRAIN_SCHEMA}}.touch_updated_at();

-- ─────────────────────────────────────────
-- 5. Knowledge collection items
-- ─────────────────────────────────────────
CREATE TABLE {{POSTBRAIN_SCHEMA}}.knowledge_collection_items (
    collection_id   UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.knowledge_collections(id) ON DELETE CASCADE,
    artifact_id     UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.knowledge_artifacts(id) ON DELETE CASCADE,
    position        INT NOT NULL DEFAULT 0,
    added_by        UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.principals(id),
    added_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (collection_id, artifact_id)
);

-- ─────────────────────────────────────────
-- 6. Sharing grants + indexes
-- ─────────────────────────────────────────
CREATE TABLE {{POSTBRAIN_SCHEMA}}.sharing_grants (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    memory_id       UUID REFERENCES {{POSTBRAIN_SCHEMA}}.memories(id) ON DELETE CASCADE,
    artifact_id     UUID REFERENCES {{POSTBRAIN_SCHEMA}}.knowledge_artifacts(id) ON DELETE CASCADE,

    grantee_scope_id UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.scopes(id) ON DELETE CASCADE,
    granted_by      UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.principals(id),
    can_reshare     BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CHECK (
        (memory_id IS NOT NULL AND artifact_id IS NULL) OR
        (memory_id IS NULL AND artifact_id IS NOT NULL)
    )
);

CREATE INDEX sharing_grants_grantee_idx  ON {{POSTBRAIN_SCHEMA}}.sharing_grants (grantee_scope_id);
CREATE INDEX sharing_grants_memory_idx   ON {{POSTBRAIN_SCHEMA}}.sharing_grants (memory_id) WHERE memory_id IS NOT NULL;
CREATE INDEX sharing_grants_artifact_idx ON {{POSTBRAIN_SCHEMA}}.sharing_grants (artifact_id) WHERE artifact_id IS NOT NULL;

-- ─────────────────────────────────────────
-- 7. Promotion requests + indexes
-- ─────────────────────────────────────────
CREATE TABLE {{POSTBRAIN_SCHEMA}}.promotion_requests (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    memory_id       UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.memories(id) ON DELETE CASCADE,
    requested_by    UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.principals(id),
    target_scope_id UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.scopes(id),
    target_visibility TEXT NOT NULL
                    CHECK (target_visibility IN ('private', 'project', 'team', 'department', 'company')),
    proposed_title  TEXT,
    proposed_collection_id UUID REFERENCES {{POSTBRAIN_SCHEMA}}.knowledge_collections(id),
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'approved', 'rejected', 'merged')),
    reviewer_id     UUID REFERENCES {{POSTBRAIN_SCHEMA}}.principals(id),
    review_note     TEXT,
    reviewed_at     TIMESTAMPTZ,
    result_artifact_id UUID REFERENCES {{POSTBRAIN_SCHEMA}}.knowledge_artifacts(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX promotion_requests_memory_idx ON {{POSTBRAIN_SCHEMA}}.promotion_requests (memory_id);
CREATE INDEX promotion_requests_status_idx ON {{POSTBRAIN_SCHEMA}}.promotion_requests (status, target_scope_id);

-- ─────────────────────────────────────────
-- 8. Staleness flags + indexes
-- ─────────────────────────────────────────
CREATE TABLE {{POSTBRAIN_SCHEMA}}.staleness_flags (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    artifact_id UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.knowledge_artifacts(id) ON DELETE CASCADE,
    signal      TEXT NOT NULL CHECK (signal IN (
                    'source_modified',
                    'contradiction_detected',
                    'low_access_age'
                )),
    confidence  FLOAT NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    evidence    JSONB NOT NULL DEFAULT '{}',
    status      TEXT NOT NULL DEFAULT 'open'
                CHECK (status IN ('open', 'dismissed', 'resolved')),
    flagged_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_by UUID REFERENCES {{POSTBRAIN_SCHEMA}}.principals(id),
    reviewed_at TIMESTAMPTZ,
    review_note TEXT
);

CREATE INDEX staleness_flags_artifact_idx ON {{POSTBRAIN_SCHEMA}}.staleness_flags (artifact_id, status);
CREATE INDEX staleness_flags_open_idx     ON {{POSTBRAIN_SCHEMA}}.staleness_flags (confidence DESC, flagged_at DESC)
    WHERE status = 'open';

-- ─────────────────────────────────────────
-- 9. Consolidations audit log
-- ─────────────────────────────────────────
CREATE TABLE {{POSTBRAIN_SCHEMA}}.consolidations (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    scope_id        UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.scopes(id) ON DELETE CASCADE,
    source_ids      UUID[] NOT NULL,
    result_id       UUID REFERENCES {{POSTBRAIN_SCHEMA}}.memories(id),
    strategy        TEXT NOT NULL,
    reason          TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ─────────────────────────────────────────
-- 10. touch_updated_at trigger for knowledge_artifacts
-- ─────────────────────────────────────────
CREATE TRIGGER knowledge_artifacts_updated_at BEFORE UPDATE ON {{POSTBRAIN_SCHEMA}}.knowledge_artifacts
    FOR EACH ROW EXECUTE FUNCTION {{POSTBRAIN_SCHEMA}}.touch_updated_at();

-- ─────────────────────────────────────────
-- 11. Forward FK: memories.promoted_to → knowledge_artifacts
-- ─────────────────────────────────────────
ALTER TABLE {{POSTBRAIN_SCHEMA}}.memories
    ADD CONSTRAINT memories_promoted_to_fk
    FOREIGN KEY (promoted_to) REFERENCES {{POSTBRAIN_SCHEMA}}.knowledge_artifacts(id) ON DELETE SET NULL;

-- ─────────────────────────────────────────
-- 12. pg_cron: detect stale knowledge (monthly)
-- ─────────────────────────────────────────
SELECT cron.schedule('detect-stale-knowledge-age', '0 6 1 * *', $$
    INSERT INTO staleness_flags (artifact_id, signal, confidence, evidence)
    SELECT
        ka.id,
        'low_access_age',
        0.3,
        jsonb_build_object(
            'last_accessed',     ka.last_accessed,
            'days_since_access', EXTRACT(EPOCH FROM (now() - COALESCE(ka.last_accessed, ka.created_at))) / 86400,
            'artifact_age_days', EXTRACT(EPOCH FROM (now() - ka.created_at)) / 86400
        )
    FROM knowledge_artifacts ka
    WHERE ka.status = 'published'
    AND   ka.created_at < now() - INTERVAL '180 days'
    AND   COALESCE(ka.last_accessed, ka.created_at) < now() - INTERVAL '60 days'
    AND   NOT EXISTS (
              SELECT 1 FROM staleness_flags sf
              WHERE  sf.artifact_id = ka.id
              AND    sf.signal       = 'low_access_age'
              AND    sf.status       = 'open'
          )
$$);

-- ─────────────────────────────────────────
-- 13. Partial index for draft/in_review knowledge
-- ─────────────────────────────────────────
CREATE INDEX knowledge_status_idx ON {{POSTBRAIN_SCHEMA}}.knowledge_artifacts (status, owner_scope_id)
    WHERE status IN ('draft', 'in_review');
