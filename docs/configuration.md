# Configuration

This page explains each Postbrain config section in user-facing terms.

Postbrain reads YAML config (for example `config.yaml`). Most keys can also be overridden with `POSTBRAIN_...`
environment variables.

## Top-level sections

- `database`: PostgreSQL connection and pool settings.
- `embedding`: model backend and embedding/analysis behavior.
- `server`: network listener and optional TLS paths.
- `migrations`: startup schema version guard behavior.
- `jobs`: background maintenance and quality jobs.
- `oauth`: social login and OAuth server settings.

## `database`

| Key                        | What it controls                                              |
|----------------------------|---------------------------------------------------------------|
| `database.url`             | PostgreSQL connection string.                                 |
| `database.auto_migrate`    | Whether Postbrain auto-applies pending migrations at startup. |
| `database.max_open`        | Max open database connections.                                |
| `database.max_idle`        | Max idle database connections.                                |
| `database.connect_timeout` | Timeout for initial DB connection.                            |

## `embedding`

| Key                         | What it controls                                   |
|-----------------------------|----------------------------------------------------|
| `embedding.backend`         | Which embedding provider backend to use.           |
| `embedding.service_url`     | Backend service URL (Ollama or OpenAI-compatible). |
| `embedding.text_model`      | Text embedding model slug.                         |
| `embedding.code_model`      | Code embedding model slug.                         |
| `embedding.summary_model`   | Optional summarize/analyze model override.         |
| `embedding.openai_api_key`  | API key for OpenAI-backed embedding/analyze paths. |
| `embedding.providers`       | Named provider runtime profiles for model routing. |
| `embedding.request_timeout` | Timeout for embedding/analyze requests.            |
| `embedding.batch_size`      | Batch size for embedding jobs.                     |

For how embeddings/chunks/entities are used during indexing and retrieval, see
[Indexing Model](./indexing-model.md).

When `embedding.backend` is `openai`:
- `embedding.service_url` can point to any OpenAI-compatible endpoint.
- If `embedding.service_url` is empty, Postbrain uses the default OpenAI API endpoint.
- `embedding.openai_api_key` is required when `embedding.service_url` is empty; optional for custom/local endpoints.

When `embedding.backend` is `ollama`:
- `embedding.service_url` is the Ollama base URL.
- If empty, Postbrain uses `http://localhost:11434`.

`embedding.providers` lets you define multiple runtime profiles (for example
`default`, `openai-prod`, `local-ollama`). Embedding models can then bind to a
profile via `postbrain-cli embedding-model register --provider-config <name>`.
If `embedding.providers` is omitted, Postbrain synthesizes `providers.default`
from the top-level `embedding.backend`, `embedding.service_url`, and
`embedding.openai_api_key` values.

## `server`

| Key               | What it controls                          |
|-------------------|-------------------------------------------|
| `server.addr`     | Listen address for REST/MCP/UI endpoints. |
| `server.tls_cert` | TLS certificate file path (optional).     |
| `server.tls_key`  | TLS private key file path (optional).     |

There is no `server.token` key in the current runtime schema. Authentication is done via issued bearer tokens used by
clients (`POSTBRAIN_TOKEN`).

## `migrations`

| Key                           | What it controls                                    |
|-------------------------------|-----------------------------------------------------|
| `migrations.expected_version` | Optional startup check for expected schema version. |

## `jobs`

| Key                               | What it controls                            |
|-----------------------------------|---------------------------------------------|
| `jobs.consolidation_enabled`      | Memory consolidation/summarization jobs.    |
| `jobs.contradiction_enabled`      | Contradiction detection jobs.               |
| `jobs.reembed_enabled`            | Re-embedding refresh jobs.                  |
| `jobs.age_check_enabled`          | Age/staleness jobs.                         |
| `jobs.backfill_summaries_enabled` | Summary backfill jobs for legacy artifacts. |
| `jobs.chunk_backfill_enabled`     | Chunk backfill jobs for legacy content.     |

## `oauth`

| Key                                        | What it controls                                                 |
|--------------------------------------------|------------------------------------------------------------------|
| `oauth.base_url`                           | Public Postbrain URL used for login/callback/metadata behavior.  |
| `oauth.providers.<provider>.enabled`       | Enable a social login provider.                                  |
| `oauth.providers.<provider>.client_id`     | Provider OAuth client ID.                                        |
| `oauth.providers.<provider>.client_secret` | Provider OAuth client secret.                                    |
| `oauth.providers.<provider>.scopes`        | Requested provider scopes.                                       |
| `oauth.providers.<provider>.instance_url`  | Provider base URL for self-hosted variants (mainly GitLab).      |
| `oauth.server.auth_code_ttl`               | OAuth authorization code lifetime.                               |
| `oauth.server.state_ttl`                   | OAuth state lifetime.                                            |
| `oauth.server.token_ttl`                   | Issued token lifetime (`0` keeps current non-expiring behavior). |
| `oauth.server.dynamic_registration`        | Enable/disable dynamic client registration endpoint.             |

For detailed login/setup steps, see [OAuth Logins and Configuration](./oauth-logins.md).

## Environment variable mapping

Config keys map to env vars by replacing dots with underscores and prefixing `POSTBRAIN_`.

Examples:

- `database.url` -> `POSTBRAIN_DATABASE_URL`
- `embedding.service_url` -> `POSTBRAIN_EMBEDDING_SERVICE_URL`
- `oauth.server.state_ttl` -> `POSTBRAIN_OAUTH_SERVER_STATE_TTL`
