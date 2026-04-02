-- Migration 000011: OAuth social identities + authorization server state.

-- 1. Social identity links.
CREATE TABLE social_identities (
    id             UUID        PRIMARY KEY DEFAULT uuidv7(),
    principal_id   UUID        NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
    provider       TEXT        NOT NULL,
    provider_id    TEXT        NOT NULL,
    email          citext,
    display_name   TEXT,
    avatar_url     TEXT,
    raw_profile    JSONB       NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_id)
);

CREATE INDEX social_identities_principal_idx ON social_identities (principal_id);

CREATE TRIGGER social_identities_updated_at BEFORE UPDATE ON social_identities
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- 2. OAuth clients.
CREATE TABLE oauth_clients (
    id                  UUID        PRIMARY KEY DEFAULT uuidv7(),
    client_id           TEXT        NOT NULL UNIQUE,
    client_secret_hash  TEXT,
    name                TEXT        NOT NULL,
    redirect_uris       TEXT[]      NOT NULL DEFAULT '{}',
    grant_types         TEXT[]      NOT NULL DEFAULT '{"authorization_code"}',
    scopes              TEXT[]      NOT NULL DEFAULT '{}',
    is_public           BOOLEAN     NOT NULL DEFAULT true,
    meta                JSONB       NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at          TIMESTAMPTZ
);

CREATE INDEX oauth_clients_client_id_idx ON oauth_clients (client_id);

-- 3. OAuth authorization codes.
CREATE TABLE oauth_auth_codes (
    id               UUID        PRIMARY KEY DEFAULT uuidv7(),
    code_hash        TEXT        NOT NULL UNIQUE,
    client_id        UUID        NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
    principal_id     UUID        NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
    redirect_uri     TEXT        NOT NULL,
    scopes           TEXT[]      NOT NULL DEFAULT '{}',
    code_challenge   TEXT        NOT NULL,
    expires_at       TIMESTAMPTZ NOT NULL,
    used_at          TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX oauth_auth_codes_code_hash_idx ON oauth_auth_codes (code_hash);
CREATE INDEX oauth_auth_codes_expires_idx ON oauth_auth_codes (expires_at)
    WHERE used_at IS NULL;

-- 4. OAuth states.
CREATE TABLE oauth_states (
    id          UUID        PRIMARY KEY DEFAULT uuidv7(),
    state_hash  TEXT        NOT NULL UNIQUE,
    kind        TEXT        NOT NULL CHECK (kind IN ('social', 'mcp_consent')),
    payload     JSONB       NOT NULL DEFAULT '{}',
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX oauth_states_state_hash_idx ON oauth_states (state_hash);
CREATE INDEX oauth_states_expires_idx ON oauth_states (expires_at)
    WHERE used_at IS NULL;
