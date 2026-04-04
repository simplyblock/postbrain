# API Auth Examples

All protected API calls require a bearer token.

These examples show the minimum flow: set environment variables, send token-authenticated requests, and keep scope
boundaries explicit.

## Environment variables

```bash
export POSTBRAIN_URL="http://localhost:7433"
export POSTBRAIN_TOKEN="<token>"
```

## Basic authenticated call

Use this as your first connectivity/auth check:

```bash
curl -sS -H "Authorization: Bearer ${POSTBRAIN_TOKEN}" \
  "${POSTBRAIN_URL}/v1/principals"
```

## Create token (server CLI)

Create dedicated tokens per workload instead of sharing one long-lived token across systems:

```bash
postbrain token create --name "automation" --principal "acme-platform"
```

## Scope-aware usage pattern

When using clients/agents, set a default scope and keep tokens least-privilege.

```bash
export POSTBRAIN_SCOPE="project:your-org/your-repo"
```

This reduces accidental cross-scope access and keeps automation behavior predictable.
