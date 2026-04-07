# Postbrain

Long-term memory for AI coding agents and the developers who work alongside them.

Postbrain gives agents (Claude Code, OpenAI Codex, GitHub Copilot, and others) a persistent, queryable store that survives session termination and context resets.

It exposes three layers backed by PostgreSQL:

| Layer | What it stores | Default visibility |
|---|---|---|
| Memory | Automatically captured observations from agent sessions | Private to scope |
| Knowledge | Curated, reviewed, versioned artifacts | Explicit on publish |
| Skills | Versioned prompt templates agents can install/invoke | Explicit on publish |

## Quick Start

Prerequisites:

- PostgreSQL 18 with required extensions (`pgvector`, `apache_age`, `pg_cron`, `pg_partman`)
- Go 1.23+
- Embedding provider (for example Ollama or OpenAI-compatible endpoint)

Then:

```bash
# 1) Start local Postgres (dev image with required extensions)
docker compose up -d postgres

# 2) Create config and adjust values as needed
cp config.example.yaml config.yaml

# 3) Build binaries
make build

# 4) Bootstrap admin principal + initial token
./postbrain onboard --config ./config.yaml --slug admin --display-name "Postbrain Admin" --token-name "bootstrap-admin"

# 5) Run server
./postbrain --config ./config.yaml
```

Default server address is `http://localhost:7433`.

## Documentation

Use `README.md` for orientation; use `docs/` for complete guidance.

- [Documentation Home](./docs/README.md)
- [Getting Started](./docs/getting-started.md)
- [Server Installation](./docs/server-installation.md)
- [Configuration Reference](./docs/configuration.md)
- [Using with Coding Agents](./docs/using-with-coding-agents.md)
- [Common Workflows](./docs/common-workflows.md)
- [Web UI Guide](./docs/webui-guide.md)
- [Security and Access Model](./docs/security.md)
- [Access Control Reference](./docs/access-control-reference.md)
- [Operations](./docs/operations.md)
- [Troubleshooting Playbook](./docs/troubleshooting-playbook.md)

## API and MCP

- REST API auth and examples: [docs/api-auth-examples.md](./docs/api-auth-examples.md)
- MCP usage patterns: [docs/common-workflows.md](./docs/common-workflows.md)
- OAuth login/server setup: [docs/oauth-logins.md](./docs/oauth-logins.md)

## Repository Layout

```text
postbrain/
├── cmd/                      # server + CLI entrypoints
├── internal/                 # application packages (api, db, authz, memory, knowledge, skills, ui)
├── docs/                     # canonical user/operator documentation
├── designs/                  # architecture and task tracking docs
├── config.example.yaml       # reference configuration
└── docker-compose.yml        # local dev stack
```

## For Agents

Before making changes, read these in order:

1. `AGENTS.md` (mandatory coding and workflow rules)
2. `docs/index.md` (starting point for selecting additional `designs/` documents to read in full)
3. `designs/DESIGN.md` (high-level architecture overview and invariants)
4. subsystem-specific design/task files identified from `docs/index.md` (for example permissions, OAuth, code graph)
5. `designs/TASKS.md` (task tracking, must be updated before commit)
