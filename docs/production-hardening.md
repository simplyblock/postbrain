# Production Hardening Checklist

Use this checklist before exposing Postbrain publicly. The goal is not maximum complexity, but a secure baseline that
is easy to maintain.

## Network and transport

Protect traffic first:

- Enable TLS (`server.tls_cert`, `server.tls_key`) or terminate TLS upstream.
- Restrict public network exposure to required endpoints.
- Apply network policies where available.

## Authentication and authorization

Treat tokens and scopes as your primary safety boundary:

- Use scoped tokens with least privilege.
- Rotate tokens regularly.
- Limit social login domains when applicable.

## Secrets

Configuration is safe to store in files; secrets are not:

- Store API keys and OAuth secrets in secret managers/Kubernetes secrets.
- Avoid plaintext secrets in git-tracked config files.

## Runtime security

Run with locked-down defaults and explicit resources:

- Run containers as non-root (default image supports this).
- Keep base images and dependencies updated.
- Use resource requests/limits for predictable behavior.

## Database security

Database compromise is system compromise, so keep DB controls strict:

- Use least-privilege DB credentials.
- Restrict DB access to trusted networks/workloads.
- Keep PostgreSQL and extensions patched.
