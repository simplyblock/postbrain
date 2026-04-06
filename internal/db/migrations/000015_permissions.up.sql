-- Migration 000015: permissions system — scope_grants table and systemadmin flag

ALTER TABLE principals
    ADD COLUMN is_system_admin BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE scope_grants (
    id           UUID PRIMARY KEY DEFAULT uuidv7(),
    principal_id UUID NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
    scope_id     UUID NOT NULL REFERENCES scopes(id) ON DELETE CASCADE,
    permissions  TEXT[] NOT NULL,
    granted_by   UUID REFERENCES principals(id) ON DELETE SET NULL,
    expires_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (principal_id, scope_id)
);

CREATE INDEX scope_grants_principal_idx ON scope_grants (principal_id);
CREATE INDEX scope_grants_scope_idx ON scope_grants (scope_id);
