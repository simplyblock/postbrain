-- Migration 000004: skills registry
-- Creates skills, skill_endorsements, skill_history tables, and wires
-- the invocation stats trigger on the events partitioned table.

-- ─────────────────────────────────────────
-- 1. Skills table + indexes
-- ─────────────────────────────────────────
CREATE TABLE skills (
    id                 UUID PRIMARY KEY DEFAULT uuidv7(),
    scope_id           UUID NOT NULL REFERENCES scopes(id) ON DELETE CASCADE,
    author_id          UUID NOT NULL REFERENCES principals(id),
    source_artifact_id UUID REFERENCES knowledge_artifacts(id) ON DELETE SET NULL,

    slug               citext NOT NULL,
    name               TEXT NOT NULL,
    description        TEXT NOT NULL,

    agent_types        TEXT[] NOT NULL DEFAULT '{"any"}',

    body               TEXT NOT NULL,
    parameters         JSONB NOT NULL DEFAULT '[]',

    visibility         TEXT NOT NULL DEFAULT 'team'
                       CHECK (visibility IN ('private','project','team','department','company')),
    status             TEXT NOT NULL DEFAULT 'draft'
                       CHECK (status IN ('draft','in_review','published','deprecated')),
    published_at       TIMESTAMPTZ,
    deprecated_at      TIMESTAMPTZ,
    review_required    INT NOT NULL DEFAULT 1,

    version            INT NOT NULL DEFAULT 1,
    previous_version   UUID REFERENCES skills(id),

    embedding          vector(1536),
    embedding_model_id UUID REFERENCES embedding_models(id),

    invocation_count   INT NOT NULL DEFAULT 0,
    last_invoked_at    TIMESTAMPTZ,

    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (scope_id, slug)
);

CREATE INDEX skills_embedding_hnsw_idx
    ON skills USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

CREATE INDEX skills_scope_status_idx
    ON skills (scope_id, status, visibility);

CREATE INDEX skills_content_fts_idx
    ON skills USING GIN (to_tsvector('postbrain_fts', description || ' ' || body));

-- ─────────────────────────────────────────
-- 2. Skill endorsements
-- ─────────────────────────────────────────
CREATE TABLE skill_endorsements (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    skill_id    UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    endorser_id UUID NOT NULL REFERENCES principals(id),
    note        TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (skill_id, endorser_id)
);

-- ─────────────────────────────────────────
-- 3. Skill version history
-- ─────────────────────────────────────────
CREATE TABLE skill_history (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    skill_id    UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    version     INT NOT NULL,
    body        TEXT NOT NULL,
    parameters  JSONB NOT NULL,
    changed_by  UUID NOT NULL REFERENCES principals(id),
    change_note TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (skill_id, version)
);

-- ─────────────────────────────────────────
-- 4. touch_updated_at trigger for skills
-- ─────────────────────────────────────────
CREATE TRIGGER skills_updated_at BEFORE UPDATE ON skills
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- ─────────────────────────────────────────
-- 5. Invocation stats trigger on events
-- NOTE: Trigger on parent partitioned table; requires PG13+.
-- ─────────────────────────────────────────
CREATE OR REPLACE FUNCTION skills_update_invocation_stats()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.event_type = 'skill_invoked' THEN
        UPDATE skills
        SET invocation_count  = invocation_count + 1,
            last_invoked_at   = NEW.created_at
        WHERE id = (NEW.payload->>'skill_id')::uuid;
    END IF;
    RETURN NEW;
END;
$$;

-- NOTE: Trigger on parent partitioned table; requires PG13+.
-- PostgreSQL 13+ propagates AFTER INSERT row-level triggers to all partitions.
-- Verify this behaviour when upgrading PostgreSQL major versions.
CREATE TRIGGER events_skill_stats
    AFTER INSERT ON events
    FOR EACH ROW EXECUTE FUNCTION skills_update_invocation_stats();
