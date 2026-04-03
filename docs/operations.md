# Operations and Troubleshooting

## Day-1 operations checklist

- verify DB connectivity and migrations
- confirm embedding backend connectivity
- validate token auth from at least one client
- run scope discovery (`list_scopes`) from agent tooling
- test `remember` + `recall` end to end

## Basic health checks

- server reachable on configured `server.addr`
- migrations applied to expected version
- background jobs enabled as intended
- logs show no repeated authz or embedding failures

## Common issues

### 1. "scope access denied"

Likely causes:

- token does not include requested scope
- principal effective scopes do not include requested scope
- endpoint/tool has stricter object-scope checks than caller expects

Actions:

- verify requested scope string
- verify token scope claims
- verify principal membership chain

### 2. Embedding errors or empty recall

Likely causes:

- embedding backend unavailable
- model slug mismatch
- request timeout too low

Actions:

- verify backend URL/API key
- verify configured model names
- increase `embedding.request_timeout` if required

### 3. OAuth login failures

Likely causes:

- bad `oauth.base_url`
- redirect URI mismatch in provider app settings
- expired/invalid state or auth code TTL settings

Actions:

- check provider callback URIs
- verify `oauth.server.state_ttl` and `auth_code_ttl`
- check server logs for provider-specific status codes

### 4. Skill sync or install issues

Likely causes:

- missing `POSTBRAIN_TOKEN`
- wrong scope
- target project path mismatch

Actions:

- verify env vars (`POSTBRAIN_URL`, `POSTBRAIN_TOKEN`)
- run with explicit `--scope` and `--target`

## Recommended maintenance

- run regular DB backups
- monitor long-running query performance
- periodically review stale or low-value memories
- promote durable decisions into knowledge artifacts

## Useful commands

```bash
make build
make test
make test-integration
make lint

postbrain-cli skill sync --scope project:your-org/your-repo --agent claude-code
postbrain-cli install-codex-skill --target /path/to/project
postbrain-cli install-claude-skill --target /path/to/project
```
