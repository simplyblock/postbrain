-- Migration 000001: initial schema
-- Creates extensions, FTS config, embedding_models, principals, tokens,
-- principal_memberships, scopes, sessions, and events (partitioned).

-- ─────────────────────────────────────────
-- 1. Extensions
-- ─────────────────────────────────────────
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS btree_gin;
CREATE EXTENSION IF NOT EXISTS ltree;
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS unaccent;
CREATE EXTENSION IF NOT EXISTS fuzzystrmatch;
CREATE EXTENSION IF NOT EXISTS pg_prewarm;
CREATE EXTENSION IF NOT EXISTS pg_cron;
CREATE EXTENSION IF NOT EXISTS pg_partman;

-- ─────────────────────────────────────────
-- 2. Custom FTS configuration with unaccent
-- ─────────────────────────────────────────
CREATE TEXT SEARCH CONFIGURATION postbrain_fts (COPY = pg_catalog.english);
ALTER TEXT SEARCH CONFIGURATION postbrain_fts
    ALTER MAPPING FOR hword, hword_part, word
    WITH unaccent, english_stem;

-- ─────────────────────────────────────────
-- 3. Embedding model registry
-- ─────────────────────────────────────────
CREATE TABLE embedding_models (
    id               UUID PRIMARY KEY DEFAULT uuidv7(),
    slug             citext NOT NULL UNIQUE,
    dimensions       INT NOT NULL,
    content_type     TEXT NOT NULL DEFAULT 'text' CHECK (content_type IN ('text', 'code')),
    is_active        BOOLEAN NOT NULL DEFAULT false,
    description      TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Each content_type has at most one active model at a time.
CREATE UNIQUE INDEX embedding_models_active_text_idx
    ON embedding_models (is_active) WHERE is_active = true AND content_type = 'text';
CREATE UNIQUE INDEX embedding_models_active_code_idx
    ON embedding_models (is_active) WHERE is_active = true AND content_type = 'code';

-- ─────────────────────────────────────────
-- 4. touch_updated_at() function
-- ─────────────────────────────────────────
CREATE OR REPLACE FUNCTION touch_updated_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN NEW.updated_at = now(); RETURN NEW; END;
$$;

-- ─────────────────────────────────────────
-- 5. Principals + updated_at trigger
-- ─────────────────────────────────────────
CREATE TABLE principals (
    id           UUID PRIMARY KEY DEFAULT uuidv7(),
    kind         TEXT NOT NULL CHECK (kind IN ('agent', 'user', 'team', 'department', 'company')),
    slug         citext NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    meta         JSONB NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER principals_updated_at BEFORE UPDATE ON principals
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- ─────────────────────────────────────────
-- 6. API tokens
-- ─────────────────────────────────────────
CREATE TABLE tokens (
    id            UUID PRIMARY KEY DEFAULT uuidv7(),
    principal_id  UUID NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
    token_hash    TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    scope_ids     UUID[],
    permissions   TEXT[] NOT NULL DEFAULT '{"read"}',
    expires_at    TIMESTAMPTZ,
    last_used_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at    TIMESTAMPTZ
);

CREATE INDEX tokens_principal_idx ON tokens (principal_id);
CREATE INDEX tokens_hash_idx      ON tokens (token_hash);
CREATE INDEX tokens_scope_ids_idx ON tokens USING GIN (scope_ids) WHERE scope_ids IS NOT NULL;

-- ─────────────────────────────────────────
-- 7. Principal memberships
-- ─────────────────────────────────────────
CREATE TABLE principal_memberships (
    member_id   UUID NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
    parent_id   UUID NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
    role        TEXT NOT NULL DEFAULT 'member',
    granted_by  UUID REFERENCES principals(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (member_id, parent_id),
    CHECK (member_id <> parent_id)
);

CREATE INDEX principal_memberships_parent_idx ON principal_memberships (parent_id);

-- ─────────────────────────────────────────
-- 8. Scopes + indexes + compute_path trigger
-- ─────────────────────────────────────────
CREATE TABLE scopes (
    id           UUID PRIMARY KEY DEFAULT uuidv7(),
    kind         TEXT NOT NULL CHECK (kind IN ('user', 'project', 'team', 'department', 'company')),
    external_id  citext NOT NULL,
    name         TEXT NOT NULL,
    parent_id    UUID REFERENCES scopes(id),
    principal_id UUID NOT NULL REFERENCES principals(id),
    path         ltree NOT NULL,
    meta         JSONB NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (kind, external_id)
);

CREATE INDEX scopes_path_gist_idx  ON scopes USING gist (path);
CREATE INDEX scopes_path_btree_idx ON scopes USING btree (path);
CREATE INDEX scopes_parent_idx     ON scopes (parent_id) WHERE parent_id IS NOT NULL;

CREATE OR REPLACE FUNCTION scopes_compute_path()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE
    parent_path ltree;
    safe_label  TEXT;
BEGIN
    safe_label := regexp_replace(NEW.external_id, '[^a-zA-Z0-9_]', '_', 'g');

    IF NEW.parent_id IS NULL THEN
        NEW.path := CASE NEW.kind
            WHEN 'user' THEN text2ltree('user.' || safe_label)
            ELSE              text2ltree(safe_label)
        END;
    ELSE
        SELECT path INTO parent_path FROM scopes WHERE id = NEW.parent_id;
        NEW.path := parent_path || text2ltree(safe_label);
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER scopes_path_trigger
    BEFORE INSERT OR UPDATE OF parent_id, external_id ON scopes
    FOR EACH ROW EXECUTE FUNCTION scopes_compute_path();

-- ─────────────────────────────────────────
-- 9. Sessions
-- ─────────────────────────────────────────
CREATE TABLE sessions (
    id           UUID PRIMARY KEY DEFAULT uuidv7(),
    scope_id     UUID NOT NULL REFERENCES scopes(id) ON DELETE CASCADE,
    principal_id UUID REFERENCES principals(id) ON DELETE SET NULL,
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at     TIMESTAMPTZ,
    meta         JSONB NOT NULL DEFAULT '{}'
);

-- ─────────────────────────────────────────
-- 10. Events (partitioned by month)
-- ─────────────────────────────────────────
CREATE TABLE events (
    id          UUID        NOT NULL DEFAULT uuidv7(),
    session_id  UUID        NOT NULL,
    scope_id    UUID        NOT NULL REFERENCES scopes(id) ON DELETE CASCADE,
    event_type  TEXT        NOT NULL,
    payload     JSONB       NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE INDEX events_session_idx ON events (session_id, created_at DESC);
CREATE INDEX events_scope_idx   ON events (scope_id, event_type, created_at DESC);

SELECT partman.create_parent(
    p_parent_table => 'public.events',
    p_control      => 'created_at',
    p_interval     => 'monthly',
    p_premake      => 3
);

UPDATE partman.part_config
    SET retention            = '24 months',
        retention_keep_table = true
    WHERE parent_table = 'public.events';
