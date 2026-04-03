# Getting Started

This guide helps you run Postbrain for the first time.

## 1. Pick your deployment style

### Managed database (for example Vela PostgreSQL)

Use this when your team already has managed Postgres.

You need:

- a PostgreSQL database URL for Postbrain
- required extensions available in that database (`vector`, `pg_trgm`, `uuid-ossp`)
- a Postbrain service process running with that database URL

### Local/custom installation

Use this for local development and self-managed environments.

Typical flow:

```bash
docker compose up -d postgres
cp config.example.yaml config.yaml
make build
./postbrain serve --config config.yaml
```

By default, the server listens on `http://localhost:7433`.

## 2. Create API tokens

Postbrain APIs require bearer tokens.

Create/manage tokens with server CLI commands:

- `postbrain token create`
- `postbrain token list`
- `postbrain token revoke`

Then set client-side environment variables:

- `POSTBRAIN_URL` (for example `http://localhost:7433`)
- `POSTBRAIN_TOKEN` (issued bearer token)
- optional `POSTBRAIN_SCOPE` for default scope selection

## 3. Install initial skills

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

## 4. Optional: enable OAuth/social login

If you want browser login and OAuth client integrations:

- configure `oauth.base_url`
- enable one or more providers under `oauth.providers`
- verify callback URLs in provider app settings
- review OAuth server settings under `oauth.server`

See [OAuth Logins and Configuration](./oauth-logins.md).

## 5. Validate setup quickly

After startup:

1. open `GET /.well-known/oauth-authorization-server` (optional but useful)
2. run one `remember` write from your client/agent
3. run one `recall` query in the same scope

If both succeed, your base setup is working.

## 6. Learn by example

See [Common Usage Workflows](./common-workflows.md) for practical patterns you can adopt immediately.
