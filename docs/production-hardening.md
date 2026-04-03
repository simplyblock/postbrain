# Production Hardening Checklist

Use this checklist before exposing Postbrain publicly.

## Network and transport

- Enable TLS (`server.tls_cert`, `server.tls_key`) or terminate TLS upstream.
- Restrict public network exposure to required endpoints.
- Apply network policies where available.

## Authentication and authorization

- Use scoped tokens with least privilege.
- Rotate tokens regularly.
- Limit social login domains when applicable.

## Secrets

- Store API keys and OAuth secrets in secret managers/Kubernetes secrets.
- Avoid plaintext secrets in git-tracked config files.

## Runtime security

- Run containers as non-root (default image supports this).
- Keep base images and dependencies updated.
- Use resource requests/limits for predictable behavior.

## Database security

- Use least-privilege DB credentials.
- Restrict DB access to trusted networks/workloads.
- Keep PostgreSQL and extensions patched.
