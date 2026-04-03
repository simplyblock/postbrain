# Security and Access Model

## Authentication

Postbrain endpoints require bearer tokens.

- REST and MCP calls are authenticated by token.
- Agent-side tooling typically uses `POSTBRAIN_TOKEN`.
- Tokens should be rotated and scoped by least privilege.

## Authorization

Authorization is scope-based and principal-aware.

- Every write/read path is expected to enforce scope checks.
- Effective scopes can include multi-hop principal relationships (for example user -> team -> company).
- Out-of-scope object access should fail with explicit authorization errors.

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
