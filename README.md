# Postbrain

Long-term memory for AI coding agents and the developers who work alongside
them, built on PostgreSQL and `pg_vector`.

Postbrain gives agents (Claude Code, OpenAI Codex, GitHub Copilot, …) a
persistent, queryable store that survives session termination, context window
resets, and agent upgrades. It exposes three layers:

| Layer | What it stores | Default visibility |
|-------|---------------|-------------------|
| **Memory** | Automatically captured observations from agent sessions | Private to scope |
| **Knowledge** | Curated, reviewed, versioned artifacts published to a team or company | Explicit on publish |
| **Skills** | Versioned, parameterised prompt templates agents can install and invoke | Explicit on publish |

All three layers are queryable together through a single MCP server or REST
API and are backed by a single PostgreSQL instance.

---

## Prerequisites

- **PostgreSQL 18** with `pg_vector`, `pg_cron`, `pg_partman` installed
  (the provided `docker-compose.yml` handles this)
- **Go 1.23+**
- An embedding backend: [Ollama](https://ollama.ai) (local, no API key) or
  an OpenAI API key

---

## Quick Start

```bash
# 1. Start PostgreSQL (pg_vector image with pg_cron + pg_partman)
docker compose up -d postgres

# 2. Copy and edit config
cp config.example.yaml config.yaml
$EDITOR config.yaml          # set database.url and embedding backend

# 3. Build
go build ./cmd/postbrain ./cmd/postbrain-hook

# 4. Run (applies pending migrations automatically, then starts serving)
./postbrain --config config.yaml
```

The server starts on `http://localhost:7433` by default.

---

## Configuration

```yaml
database:
  url:          "postgres://postbrain:postbrain@localhost:5432/postbrain"
  auto_migrate: true          # apply pending migrations on startup

embedding:
  backend:      ollama        # ollama | openai
  ollama_url:   "http://localhost:11434"
  text_model:   "nomic-embed-text"   # 768-dim general text
  code_model:   "nomic-embed-code"   # code fragments (optional)
  # openai_api_key: sk-...    # required when backend = openai

server:
  addr:  ":7433"
  token: "changeme"           # Bearer token for all API calls
```

---

## Connecting Claude Code

Add to `~/.claude/settings.json` or `.claude/settings.json` in a project:

```json
{
  "mcpServers": {
    "postbrain": {
      "type": "sse",
      "url": "http://localhost:7433/mcp",
      "headers": { "Authorization": "Bearer changeme" }
    }
  }
}
```

Claude Code then has access to `remember`, `recall`, `forget`, `context`,
`summarize`, `publish`, `endorse`, `promote`, `collect`, `skill_search`,
`skill_install`, and `skill_invoke` as native tools.

### Recommended hooks

```json
{
  "hooks": {
    "PostToolUse": [{
      "matcher": "Edit|Write|Bash",
      "command": "postbrain-hook snapshot --scope project:$POSTBRAIN_SCOPE"
    }],
    "Stop": [{
      "command": "postbrain-hook summarize-session --scope project:$POSTBRAIN_SCOPE"
    }]
  }
}
```

### Sync skills on session start

```bash
postbrain-hook skill sync --scope project:$POSTBRAIN_SCOPE --agent claude-code
```

---

## Schema Migrations

Migrations are embedded in the binary and applied automatically at startup
when `database.auto_migrate: true`. Manual control:

```bash
./postbrain migrate status      # show applied / pending migrations
./postbrain migrate up          # apply all pending
./postbrain migrate down 1      # roll back one step (dev/recovery only)
./postbrain migrate version     # print current schema version
./postbrain migrate force N     # reset dirty state after a failed migration
```

---

## Repository Layout

```
postbrain/
├── cmd/
│   ├── postbrain/          # main server binary
│   └── postbrain-hook/     # CLI helper for agent hooks and skill sync
├── internal/
│   ├── api/mcp/            # MCP server and tool handlers
│   ├── api/rest/           # REST API handlers
│   ├── db/                 # pgx pool, migration runner, sqlc queries
│   │   └── migrations/     # embedded SQL migration files (000001_…)
│   ├── embedding/          # text and code embedding backends
│   ├── memory/             # memory CRUD, hybrid retrieval, consolidation
│   ├── knowledge/          # knowledge artifact CRUD, visibility, lifecycle
│   ├── skills/             # skill registry, install, sync
│   ├── retrieval/          # cross-layer merge and re-rank
│   ├── graph/              # entity/relation store, Apache AGE sync
│   ├── principals/         # principal and membership management
│   ├── sharing/            # sharing grant enforcement
│   └── jobs/               # background jobs scheduler
├── AGENTS.md               # ← agent rules (read first)
├── DESIGN.md               # ← full architecture and design decisions
├── TASKS.md                # ← current task list and status
├── config.example.yaml
└── docker-compose.yml
```

---

## For Agents — Read This Before Implementing Anything

Before writing a single line of code, read these files **in order**:

1. **`AGENTS.md`** — mandatory rules and guard rails: TDD workflow,
   commit discipline, what you must never do. No exceptions.

2. **`DESIGN.md`** — the full architecture. Understand the three-layer model,
   the database schema, the scope hierarchy, and the extension rationale
   before touching any code. Do **not** change the design without asking first.

3. **`TASKS.md`** — the current task list. Find your assigned task, understand
   its scope, and update this file before every commit.

### Key constraints (summary — full rules in `AGENTS.md`)

- **TDD**: write the test before the implementation.
- **Test suite**: `go test ./...` must pass before every commit.
- **Formatter**: `gofmt -w .` before every commit.
- **Commits**: after each task; detailed message; `Co-authored-by:` trailer required.
- **DESIGN.md**: do not edit without asking and explaining why.
- **No unsolicited refactoring**: mark with `TODO` and move on.
- **No heavy dependencies**: check `DESIGN.md` for the approved stack first.
