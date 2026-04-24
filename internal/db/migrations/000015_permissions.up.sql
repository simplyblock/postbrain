-- Migration 000015: permissions system — scope_grants table and systemadmin flag

ALTER TABLE {{POSTBRAIN_SCHEMA}}.principals
    ADD COLUMN is_system_admin BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE {{POSTBRAIN_SCHEMA}}.scope_grants (
    id           UUID PRIMARY KEY DEFAULT uuidv7(),
    principal_id UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.principals(id) ON DELETE CASCADE,
    scope_id     UUID NOT NULL REFERENCES {{POSTBRAIN_SCHEMA}}.scopes(id) ON DELETE CASCADE,
    permissions  TEXT[] NOT NULL,
    granted_by   UUID REFERENCES {{POSTBRAIN_SCHEMA}}.principals(id) ON DELETE SET NULL,
    expires_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (principal_id, scope_id)
);

CREATE INDEX scope_grants_principal_idx ON {{POSTBRAIN_SCHEMA}}.scope_grants (principal_id);
CREATE INDEX scope_grants_scope_idx ON {{POSTBRAIN_SCHEMA}}.scope_grants (scope_id);
