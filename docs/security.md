# Security and Access Model

For full permission matrices and detailed access semantics, see
[Access Control Reference](./access-control-reference.md).

## Authentication

Postbrain endpoints require bearer tokens.

- REST and MCP calls are authenticated by token.
- Agent-side tooling typically uses `POSTBRAIN_TOKEN`.
- Tokens should be rotated and scoped by least privilege.

## Authorization

Authorization is scope-based and principal-aware. Effective permissions are evaluated as:

- `(principal, resource:operation, scope)`
- Example permission keys: `memories:read`, `knowledge:write`, `scopes:edit`, `tokens:delete`.
- Shorthand permissions `read`, `write`, `edit`, and `delete` expand across all resources.

Permissions are derived additively from:

- principal `is_system_admin` flag (full bypass)
- direct scope ownership (full access on owned scopes and descendants)
- membership role inheritance (`member`, `admin`, `owner`)
- explicit `scope_grants` on specific scopes

Inheritance rules:

- downward inheritance always applies (parent scope grants apply to descendants)
- upward inheritance is read-only (read on a child implies read on ancestors)
- no upward `write`, `edit`, or `delete`

Out-of-scope or out-of-permission object access should fail with explicit authorization errors.

## Token downscoping and self-service

Tokens are never stronger than their principal. At request time, access is the intersection of:

- principal effective permissions on the target scope
- token declared permissions
- token scope restrictions (`scope_ids`, if present)

Token self-service is always allowed for a principal's own tokens (create/list/update scopes/revoke). Managing tokens
for other principals requires elevated authority (`principals:edit` on the relevant authority scope, or system admin).

## Scope design recommendations

- Treat scopes as tenancy boundaries.
- Use one project scope per codebase.
- Put shared guidance in higher scopes only when intended.
- Avoid giving broad company scope tokens to narrow automation tasks.

## Data handling guidance

- Do not store secrets in memory/knowledge content.
- Prefer references to secret locations instead of raw values.
- Keep PII in least-shared scopes and with stricter retention policies.

## Operational controls

- Monitor denied authorization events and anomaly patterns.
- Keep dependencies and Postgres extensions patched.
- Restrict network exposure of Postbrain server where possible.
- Use TLS for non-local deployments (`server.tls_cert` + `server.tls_key`).

## OAuth and social login

If OAuth/social login is enabled:

- set explicit redirect/base URLs
- use provider-specific least-scope settings
- protect client secrets in deployment secrets managers

See [Configuration Reference](./configuration.md) for OAuth keys.
