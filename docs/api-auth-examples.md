# API Auth Examples

All protected API calls require a bearer token.

## Environment variables

```bash
export POSTBRAIN_URL="http://localhost:7433"
export POSTBRAIN_TOKEN="<token>"
```

## Basic authenticated call

```bash
curl -sS -H "Authorization: Bearer ${POSTBRAIN_TOKEN}" \
  "${POSTBRAIN_URL}/v1/principals"
```

## Create token (server CLI)

```bash
postbrain token create --name "automation"
```

## Scope-aware usage pattern

When using clients/agents, set a default scope and keep tokens least-privilege.

```bash
export POSTBRAIN_SCOPE="project:your-org/your-repo"
```
