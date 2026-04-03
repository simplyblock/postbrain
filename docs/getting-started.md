# Getting Started

This guide helps you run Postbrain for the first time.

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

Create/manage tokens with server CLI commands:

- `postbrain token create`
- `postbrain token list`
- `postbrain token revoke`

Then set client-side environment variables:

- `POSTBRAIN_URL` (for example `http://localhost:7433`)
- `POSTBRAIN_TOKEN` (issued bearer token)
- optional `POSTBRAIN_SCOPE` for default scope selection

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

## 7. Learn by example

See [Common Usage Workflows](./common-workflows.md) for practical patterns you can adopt immediately.
