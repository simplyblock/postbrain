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
| `embedding.providers`       | Named provider runtime profiles for model routing. |
| `embedding.providers.<name>.backend` | Provider backend (`ollama` or `openai`). |
| `embedding.providers.<name>.service_url` | Provider endpoint URL. |
| `embedding.providers.<name>.api_key` | Provider API key (for OpenAI-compatible services). |
| `embedding.providers.<name>.text_model` | Provider-specific text embedding model slug. |
| `embedding.providers.<name>.code_model` | Provider-specific code embedding model slug. |
| `embedding.providers.<name>.summary_model` | Provider-specific summarize/analyze model slug used by model-driven summarizers. |
| `embedding.request_timeout` | Timeout for embedding/analyze requests.            |
| `embedding.batch_size`      | Batch size for embedding jobs.                     |

For how embeddings/chunks/entities are used during indexing and retrieval, see
[Indexing Model](./indexing-model.md).

`embedding.providers` lets you define multiple runtime profiles (for example
`default`, `openai-prod`, `local-ollama`). Models bind to a profile via
`postbrain --config config.yaml embedding-model register --provider-config <name>`
or `postbrain --config config.yaml summary-model register --provider-config <name>`.

At runtime:
- text/code embeddings resolve from active `ai_models` rows where `model_type='embedding'`
- summarize/analyze resolves from active `ai_models` row where `model_type='generation'` and `content_type='text'`
- if no active generation model exists, summarize/analyze falls back to the active text embedding model profile

If model-driven lookup is unavailable, startup falls back to `embedding.providers.default`.
For `backend: openai`, `api_key` is required when `service_url` is empty (default OpenAI API URL).

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
- `embedding.providers.default.service_url` -> `POSTBRAIN_EMBEDDING_PROVIDERS_DEFAULT_SERVICE_URL`
- `oauth.server.state_ttl` -> `POSTBRAIN_OAUTH_SERVER_STATE_TTL`
