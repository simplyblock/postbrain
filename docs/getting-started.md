# Getting Started

This guide helps you run Postbrain for the first time.

## 0. Bootstrap the first system admin principal + token

For a fresh installation, use the built-in onboarding command. It runs migrations, creates (or reuses) a bootstrap
principal with system-wide admin privileges, creates an initial scope, and prints a new API token.

```bash
postbrain onboard \
  --config ./config.yaml \
  --slug admin \
  --display-name "Postbrain Admin" \
  --token-name "bootstrap-admin"
```

Store the printed token immediately, then export:

```bash
export POSTBRAIN_URL="http://localhost:7433"
export POSTBRAIN_TOKEN="<token-from-onboard>"
```

## 1. Pick your deployment style

Server installation options are documented in:

- [Server Installation](./server-installation.md)

This includes:

- local process (source build)
- GitHub release binaries
- Docker image deployment
- Kubernetes Helm deployment

## 2. Install client tooling (`postbrain-cli`)

Install `postbrain-cli` from one of these paths:

- GitHub Releases:
  - download `postbrain-client_<os>_<arch>` archive
  - extract `postbrain-cli` into your PATH
- Linux packages:
  - `.deb`: `postbrain-client_<version>_linux_<arch>.deb`
  - `.rpm`: `postbrain-client_<version>_linux_<arch>.rpm`
- Package manifests:
  - Homebrew: `packaging/homebrew/`
  - MacPorts: `packaging/macports/`
  - winget: `packaging/winget/`

Quick one-liner installer (client binary, latest release):

```bash
./scripts/install-postbrain.sh client
./scripts/install-postbrain.sh client v1.2.3
```

Verify:

```bash
postbrain-cli version
```

## 3. Create API tokens

Postbrain APIs require bearer tokens.

Create/manage tokens with server CLI commands. Tokens are downscoped credentials: each token can only exercise
permissions and scope access that the owning principal already has.

Note: `token create` requires both a token name and the owning principal slug.

- `postbrain token create --name "<name>" --principal "<principal-slug>"`
- `postbrain token list`
- `postbrain token revoke`

Then set client-side environment variables:

- `POSTBRAIN_URL` (for example `http://localhost:7433`)
- `POSTBRAIN_TOKEN` (issued bearer token)
- optional `POSTBRAIN_SCOPE` for default scope selection

Permission model reference for token design:

- permission format: `{resource}:{operation}` (for example `memories:read`, `knowledge:write`)
- shorthand permissions: `read`, `write`, `edit`, `delete` (expanded across all resources)
- scope restrictions: token `scope_ids` restrict access to selected scopes and descendants

## 4. Install initial skills

Use the CLI to install Postbrain skill files into your project.

Codex:

```bash
postbrain-cli install-codex-skill --target /path/to/project
```

Claude Code:

```bash
postbrain-cli install-claude-skill --target /path/to/project
```

If `--target` is omitted, current directory (`.`) is treated as the project root.

## 5. Optional: enable OAuth/social login

If you want browser login and OAuth client integrations:

- configure `oauth.base_url`
- enable one or more providers under `oauth.providers`
- verify callback URLs in provider app settings
- review OAuth server settings under `oauth.server`

See [OAuth Logins and Configuration](./oauth-logins.md).

## 6. Validate setup quickly

After startup:

1. open `GET /.well-known/oauth-authorization-server` (optional but useful)
2. run one `remember` write from your client/agent
3. run one `recall` query in the same scope

If both succeed, your base setup is working.

## 7. Initial organization setup (company -> team -> project -> user)

After bootstrap, create your first principal/scope hierarchy. Examples below use `curl` + `jq`.

### 7.1 Create principals

```bash
company_id="$(
  curl -sS -X POST "${POSTBRAIN_URL}/v1/principals" \
    -H "Authorization: Bearer ${POSTBRAIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{"kind":"company","slug":"acme","display_name":"ACME Inc"}' | jq -r '.ID'
)"

team_id="$(
  curl -sS -X POST "${POSTBRAIN_URL}/v1/principals" \
    -H "Authorization: Bearer ${POSTBRAIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{"kind":"team","slug":"acme-platform","display_name":"Platform Team"}' | jq -r '.ID'
)"

user_id="$(
  curl -sS -X POST "${POSTBRAIN_URL}/v1/principals" \
    -H "Authorization: Bearer ${POSTBRAIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{"kind":"user","slug":"alice","display_name":"Alice"}' | jq -r '.ID'
)"
```

### 7.2 Add membership chain

User belongs to team, team belongs to company:

```bash
curl -sS -X POST "${POSTBRAIN_URL}/v1/principals/${company_id}/members" \
  -H "Authorization: Bearer ${POSTBRAIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"member_id\":\"${team_id}\",\"role\":\"member\"}"

curl -sS -X POST "${POSTBRAIN_URL}/v1/principals/${team_id}/members" \
  -H "Authorization: Bearer ${POSTBRAIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"member_id\":\"${user_id}\",\"role\":\"member\"}"
```

### 7.3 Create scopes

```bash
company_scope_id="$(
  curl -sS -X POST "${POSTBRAIN_URL}/v1/scopes" \
    -H "Authorization: Bearer ${POSTBRAIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"kind\":\"company\",\"external_id\":\"acme\",\"name\":\"ACME\",\"principal_id\":\"${company_id}\"}" | jq -r '.ID'
)"

team_scope_id="$(
  curl -sS -X POST "${POSTBRAIN_URL}/v1/scopes" \
    -H "Authorization: Bearer ${POSTBRAIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"kind\":\"team\",\"external_id\":\"acme/platform\",\"name\":\"Platform\",\"principal_id\":\"${team_id}\",\"parent_id\":\"${company_scope_id}\"}" | jq -r '.ID'
)"

project_scope_id="$(
  curl -sS -X POST "${POSTBRAIN_URL}/v1/scopes" \
    -H "Authorization: Bearer ${POSTBRAIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"kind\":\"project\",\"external_id\":\"acme/platform/postbrain\",\"name\":\"Postbrain\",\"principal_id\":\"${team_id}\",\"parent_id\":\"${team_scope_id}\"}" | jq -r '.ID'
)"

user_scope_id="$(
  curl -sS -X POST "${POSTBRAIN_URL}/v1/scopes" \
    -H "Authorization: Bearer ${POSTBRAIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"kind\":\"user\",\"external_id\":\"alice\",\"name\":\"Alice Personal\",\"principal_id\":\"${user_id}\",\"parent_id\":\"${team_scope_id}\"}" | jq -r '.ID'
)"
```

Set default working scope for agent tooling:

```bash
export POSTBRAIN_SCOPE="project:acme/platform/postbrain"
```

### 7.4 Create a project token

```bash
postbrain token create --name "postbrain-project" --principal "acme-platform"
```

Use the returned token for CI/agents that should only work in this project context.

### 7.5 Attach a Git repository to the project scope

```bash
curl -sS -X POST "${POSTBRAIN_URL}/v1/scopes/${project_scope_id}/repo" \
  -H "Authorization: Bearer ${POSTBRAIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"repo_url":"https://github.com/simplyblock/postbrain.git","default_branch":"main"}'
```

Trigger initial indexing:

```bash
curl -sS -X POST "${POSTBRAIN_URL}/v1/scopes/${project_scope_id}/repo/sync" \
  -H "Authorization: Bearer ${POSTBRAIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{}'
```

Check indexing status:

```bash
curl -sS -H "Authorization: Bearer ${POSTBRAIN_TOKEN}" \
  "${POSTBRAIN_URL}/v1/scopes/${project_scope_id}/repo/sync"
```

### 7.6 Install and sync skills in the project

```bash
postbrain-cli install-codex-skill --target /path/to/project
postbrain-cli install-claude-skill --target /path/to/project
postbrain-cli skill sync --scope "project:acme/platform/postbrain" --agent "claude-code"
```

## 8. Learn by example

See [Common Usage Workflows](./common-workflows.md) for practical patterns you can adopt immediately.
