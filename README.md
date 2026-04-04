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

- **PostgreSQL 18** with `pg_vector`, `apache_age`, `pg_cron`, `pg_partman` installed
  (the provided `docker-compose.yml` builds a local dev image with all extensions)
- **Go 1.23+**
- An embedding backend: [Ollama](https://ollama.ai) (local, no API key) or
  an OpenAI API key

---

## Quick Start

```bash
# 1. Start PostgreSQL (local pg_vector + Apache AGE image with pg_cron + pg_partman)
docker compose up -d postgres

# 2. Copy and edit config
cp config.example.yaml config.yaml
$EDITOR config.yaml          # set database.url and embedding backend

# 3. Build
make build

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
  summary_model: ""           # model for AI-generated summaries (optional)
  # openai_api_key: sk-...    # required when backend = openai

server:
  addr:  ":7433"
  token: "changeme"           # Bearer token for all API calls

jobs:
  consolidation_enabled:      true   # merge near-duplicate memories
  contradiction_enabled:      true   # flag conflicting memories
  reembed_enabled:            true   # sync embeddings when model changes
  age_check_enabled:          true   # decay scoring and TTL expiry
  backfill_summaries_enabled: true   # AI-summarise existing artifacts
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

### Getting Started

Install the Postbrain skill into your project for the agent you use:

If `--target` is omitted, the current directory (`.`) is used as the project root.

For Codex:

```bash
postbrain-cli install-codex-skill --target /path/to/project
# or, from inside the project directory:
postbrain-cli install-codex-skill
```

For Claude Code:

```bash
postbrain-cli install-claude-skill --target /path/to/project
# or, from inside the project directory:
postbrain-cli install-claude-skill
```

You can install both when a repository is used by multiple agents.

### Recommended hooks

```json
{
  "hooks": {
    "PostToolUse": [{
      "matcher": "Edit|Write|Bash",
      "command": "postbrain-cli snapshot --scope project:$POSTBRAIN_SCOPE"
    }],
    "Stop": [{
      "command": "postbrain-cli summarize-session --scope project:$POSTBRAIN_SCOPE"
    }]
  }
}
```

### Sync skills on session start

```bash
postbrain-cli skill sync --scope project:$POSTBRAIN_SCOPE --agent claude-code
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

## MCP Tools Reference

All tools are exposed over the MCP server at `/mcp` (Streamable HTTP) and `/sse` (SSE).
Every call requires a `Bearer` token in the `Authorization` header.

Scopes are always passed as `kind:external_id` strings, e.g. `project:acme/api`
or `team:backend`. Use `list_scopes` to discover which scopes your token can access.

### Discovery

#### `list_scopes`

Lists all scopes accessible to the calling token. Use this at the start of a
session to find valid scope strings for other tools.

```
list_scopes()
→ { "scopes": [{ "id", "kind", "external_id", "name", "scope" }] }
```

Tokens with an unrestricted `scope_ids` list return all scopes. Tokens with
an explicit list return only the scopes they are authorised for. The `scope`
field is the ready-to-use `kind:external_id` string.

---

### Memory Layer

#### `remember`

Stores a new memory or updates an existing near-duplicate (cosine distance ≤ 0.05).
Near-duplicates are merged in-place rather than creating duplicates.

```
remember(
  content*,        # the observation to store
  scope*,          # kind:external_id
  memory_type,     # semantic | episodic | procedural | working (default: semantic)
  importance,      # 0–1 (default: 0.5)
  source_ref,      # provenance, e.g. "file:src/main.go:42"
  entities,        # ["EntityName", ...] — linked concept nodes
  expires_in,      # TTL in seconds; only for memory_type=working
)
→ { "memory_id", "action": "created"|"updated" }
```

**Memory types:**

| Type | Use for | Auto-expires |
|------|---------|-------------|
| `semantic` | Facts, architecture decisions, preferences | No |
| `episodic` | Time-stamped events, what happened and when | No |
| `procedural` | How-to knowledge, workflows, runbooks | No |
| `working` | Current task context, in-progress state | Yes (TTL) |

#### `recall`

Retrieves memories, knowledge artifacts, and skills relevant to a query using
hybrid vector + BM25 search. All three layers are searched simultaneously by
default.

```
recall(
  query*,          # semantic search query
  scope*,          # kind:external_id
  memory_types,    # filter: ["semantic", "episodic", ...]
  layers,          # filter: ["memory", "knowledge", "skill"] (default: all)
  agent_type,      # filter skills by agent compatibility
  limit,           # max results (default: 10)
  min_score,       # minimum combined score 0–1 (default: 0.0)
  search_mode,     # text | code | hybrid (default: hybrid)
)
→ { "results": [{ "layer", "id", "score", "content", "memory_type",
                  "knowledge_type", "full_content_available", ... }] }
```

When a knowledge result has `full_content_available: true`, only the summary
is returned. Use `knowledge_detail` to retrieve the full content.

When multiple published artifacts cover the same topic, their sources are
automatically suppressed from results if a *digest* artifact synthesised from
them is also present. Digests are created with `synthesize_topic`.

#### `forget`

Deactivates (soft-delete) or permanently removes a memory.

```
forget(
  memory_id*,      # UUID of the memory
  hard,            # true = permanent delete (default: false)
)
→ { "memory_id", "action": "deactivated"|"deleted" }
```

#### `context`

Returns a structured context bundle for the current scope and query, combining
relevant knowledge and memories within a token budget. Useful at the start of
a session to prime the agent's working context.

```
context(
  scope*,          # kind:external_id
  query,           # what you are about to work on
  max_tokens,      # token budget (default: 4000)
)
→ { "context_blocks": [{ "layer", "type", "title", "content" }],
    "total_tokens" }
```

Knowledge is prioritised over memory in the response (higher trust, higher
relevance for multi-session context).

#### `summarize`

Consolidates near-duplicate memories within a scope into single higher-level
memories, reducing noise and context window usage.

```
summarize(
  scope*,          # kind:external_id
  topic,           # cluster by topic (optional)
  dry_run,         # preview without writing (default: false)
)
→ { "consolidated_count", "result_memory_id", "summary" }
  (dry_run: { "would_consolidate": [...ids], "proposed_summary" })
```

---

### Knowledge Layer

#### `publish`

Creates a new knowledge artifact in `draft` status (or `in_review` when
`auto_review: true`). The artifact must pass the review workflow before it
becomes visible to the wider audience.

```
publish(
  title*,
  content*,
  knowledge_type*,  # semantic | episodic | procedural | reference
  scope*,           # owner scope as kind:external_id
  visibility,       # private | project | team | department | company (default: team)
  summary,          # short summary; auto-generated if omitted
  auto_review,      # skip draft, go straight to in_review (default: false)
  collection_slug,  # add to this collection after creation
)
→ { "artifact_id", "status", "version" }
```

A summary is automatically generated when `summary` is omitted:
AI-generated if a `summary_model` is configured, extractive (first ~150 words)
otherwise.

#### `knowledge_detail`

Retrieves the full content of a knowledge artifact by ID. Use this when
`recall` returns a result with `full_content_available: true` and the summary
is insufficient.

```
knowledge_detail(
  artifact_id*,    # UUID of the artifact
)
→ { "id", "title", "content", "knowledge_type", "status", "visibility", "version" }
```

#### `endorse`

Records an endorsement on a knowledge artifact or skill that is `in_review`.
When the endorsement count reaches the artifact's `review_required` threshold,
the artifact is automatically published.

```
endorse(
  artifact_id*,    # UUID of the artifact or skill
  note,            # optional endorsement note
)
→ { "artifact_id", "endorsement_count", "status", "auto_published" }
```

Rules:
- Authors cannot endorse their own artifacts (unless they are also a scope admin).
- The artifact must be `in_review`; use the REST API or a scope admin to
  submit a draft for review first.
- Endorsements are idempotent: endorsing twice has no additional effect.

#### `promote`

Nominates a memory for elevation into a persistent knowledge artifact. Creates
a promotion request that a reviewer can approve via the REST API.

```
promote(
  memory_id*,         # UUID of the memory to promote
  target_scope*,      # scope that will own the artifact: kind:external_id
  target_visibility*, # private | project | team | department | company
  proposed_title,     # suggested title for the resulting artifact
  collection_slug,    # optionally add the resulting artifact to this collection
)
→ { "promotion_request_id", "status" }
```

#### `synthesize_topic`

Synthesises multiple published knowledge artifacts into a single topic *digest*
artifact. The digest enters the normal lifecycle (draft → in_review → published);
once published, its source artifacts are suppressed from `recall` results in favour
of the digest.

```
synthesize_topic(
  scope*,          # owner scope as kind:external_id
  source_ids*,     # list of artifact UUIDs to synthesise (minimum 2,
                   # all must be published; none may themselves be digests)
  title,           # digest title; inferred from sources if omitted
  auto_review,     # skip draft, go straight to in_review (default: false)
)
→ { "artifact_id", "title", "status", "source_count" }
```

Rules:
- All source artifacts must be `published`.
- Sources must not be digests themselves.
- All source scopes must be in the lineage of the digest scope (ancestors or
  descendants); sibling scopes are blocked.
- Source artifacts remain published after synthesis; they are suppressed only
  at recall time when a covering published digest is present.

#### `collect`

Manages knowledge collections (curated bundles of artifacts).

**Actions:**

```
collect(action="create_collection",
  scope*,           # kind:external_id
  name*,            # collection display name
  collection_slug*, # URL-friendly identifier
  description,
)
→ { "collection_id", "slug", "name" }

collect(action="add_to_collection",
  artifact_id*,
  collection_id,    # UUID — use this or collection_slug+scope
  collection_slug,  # requires scope when used
  scope,            # required when using collection_slug
)
→ { "collection_id", "artifact_id" }

collect(action="list_collections",
  scope*,           # kind:external_id
)
→ { "collections": [{ "id", "slug", "name" }] }
```

---

### Skills Layer

#### `skill_search`

Searches for published skills by semantic similarity. Returns both installed
and uninstalled skills; filter with `installed: true` to see only what is
already materialised locally.

```
skill_search(
  query*,          # what you want to do
  scope,           # kind:external_id (optional — searches all visible scopes)
  agent_type,      # filter: "claude-code" | "codex" | ...
  limit,           # default: 10
  installed,       # true | false filter
)
→ { "results": [{ "id", "slug", "name", "description", "score",
                  "agent_types", "status", "invocation_count", "installed" }] }
```

#### `skill_install`

Materialises a skill into the agent's command directory
(`.claude/commands/<slug>.md` for Claude Code). After installation the skill
appears as a native slash command.

```
skill_install(
  skill_id,        # UUID — use this or slug+scope
  slug,            # requires scope when used
  scope,           # kind:external_id
  agent_type,      # default: "claude-code"
  workdir,         # installation root (default: ".")
)
→ { "path", "slug" }
```

#### `skill_invoke`

Looks up a skill by slug, substitutes the provided parameters into the body,
and returns the expanded prompt. Use this to execute a skill inline without
materialising a file.

```
skill_invoke(
  slug*,           # skill slug
  scope*,          # kind:external_id
  agent_type,      # filter by agent compatibility
  params,          # { "param_name": value, ... }
)
→ { "skill_id", "slug", "body" }
```

---

## Workflows

### 1. Session Start

At the beginning of every agent session, prime context and sync skills:

```
1. list_scopes()
     → pick the relevant scope (e.g. "project:acme/api")

2. context(scope, query="<what you are about to work on>")
     → inject the returned context_blocks into working memory

3. skill_search(query="<task description>", scope, agent_type="claude-code")
     → review suggested skills

4. skill_install(slug="<relevant-skill>", scope, agent_type="claude-code")
     → install any useful skills as slash commands
```

---

### 2. Capturing Observations (Memory)

During a session, store observations as they arise:

```
remember(content="<observation>", scope, memory_type="semantic")
  → { memory_id, action: "created" | "updated" }
```

For ephemeral task context that should not persist beyond the session:

```
remember(content="Currently refactoring the auth module",
         scope, memory_type="working", expires_in=7200)
```

After multiple sessions, consolidate accumulated memories:

```
summarize(scope, dry_run=true)   # preview
summarize(scope)                  # execute consolidation
```

---

### 3. Memory → Knowledge Promotion

When a memory contains something valuable enough to share with the team:

```
1. remember(content="...", scope)
     → { memory_id }

2. promote(memory_id,
           target_scope="team:backend",
           target_visibility="team",
           proposed_title="How we handle distributed locks")
     → { promotion_request_id, status: "pending" }

     A reviewer approves the promotion request via REST API:
     POST /v1/promotions/{id}/approve

3. The approved memory becomes a knowledge artifact (status: "draft").
   Optionally edit it via REST API, then submit for review:
   POST /v1/knowledge/{id}/submit-review

4. endorse(artifact_id, note="Confirmed correct")
     → { endorsement_count: 1, status: "in_review" }
   (repeat until endorsement_count >= review_required)
     → { auto_published: true, status: "published" }
```

---

### 4. Direct Knowledge Publishing

For content authored directly, bypassing the memory layer:

```
1. publish(title, content, knowledge_type="procedural",
           scope, visibility="team",
           auto_review=true)
     → { artifact_id, status: "in_review" }

2. endorse(artifact_id)           # first reviewer
     → { endorsement_count: 1, status: "in_review" }

3. endorse(artifact_id)           # second reviewer (if review_required=2)
     → { endorsement_count: 2, status: "published", auto_published: true }
```

To organise artifacts into collections:

```
collect(action="create_collection",
        scope, name="API Design Decisions", collection_slug="api-decisions")
  → { collection_id }

collect(action="add_to_collection",
        artifact_id, collection_slug="api-decisions", scope)

collect(action="list_collections", scope)
  → { collections: [...] }
```

---

### 5. Knowledge → Skill Promotion

Procedural knowledge artifacts can be promoted into executable skills via
the REST API:

```
POST /v1/knowledge/{artifact_id}/promote-to-skill
  body: { slug, agent_types, parameters }
  → skill is created in draft status

endorse(skill_id)   # follows same review → published lifecycle
```

Once published, agents discover and use the skill:

```
skill_search(query="deploy service", scope)
  → [{ slug: "deploy", installed: false, ... }]

skill_install(slug="deploy", scope)
  → { path: ".claude/commands/deploy.md" }

skill_invoke(slug="deploy", scope,
             params={ "environment": "staging", "service": "api" })
  → { body: "<expanded prompt>" }
```

---

### 6. Topic Synthesis

When several published artifacts cover overlapping or complementary aspects of
the same topic, synthesise them into a single digest to reduce recall noise and
token usage:

```
1. synthesize_topic(
       scope,
       source_ids=["<id-1>", "<id-2>", "<id-3>"],
       title="Authentication architecture overview",
       auto_review=true,
   )
     → { artifact_id, status: "in_review", source_count: 3 }

2. endorse(artifact_id)   # repeat until threshold is reached
     → { status: "published", auto_published: true }
```

Once the digest is published, `recall` and `context` automatically return the
digest instead of its individual sources. Source artifacts remain accessible
directly via `knowledge_detail`.

To inspect the relationship:

```
GET /v1/knowledge/{digest_id}/sources   → source artifacts
GET /v1/knowledge/{source_id}/digests   → digests covering this artifact
```

---

### 7. Recall and Retrieval

For general semantic search across all layers:

```
recall(query="how do we handle authentication", scope)
  → results from memory + knowledge + skills, ranked by relevance

recall(query="...", scope, layers=["knowledge"], min_score=0.7)
  → knowledge only, high-confidence results

recall(query="...", scope, search_mode="code")
  → optimised for code fragment queries
```

When a knowledge result has `full_content_available: true`:

```
recall(...)
  → [{ id, content: "<summary>", full_content_available: true, ... }]

knowledge_detail(artifact_id=id)
  → { content: "<full document>" }
```

---

### 8. Artifact Visibility Levels

Knowledge and skills follow a five-level visibility hierarchy:

| Level | Visible to |
|-------|-----------|
| `private` | Author only |
| `project` | Members of the owning scope |
| `team` | Members of the owning scope + parent team scope |
| `department` | Up to department level in the scope hierarchy |
| `company` | All principals in the organisation |

Visibility is set at publish time and cannot be widened without a new
version. The `recall` tool automatically filters results to what the
calling token's principal is allowed to see.

---

## Repository Layout

```
postbrain/
├── cmd/
│   ├── postbrain/          # main server binary
│   └── postbrain-cli/      # CLI helper for agent hooks and skill sync
├── internal/
│   ├── api/mcp/            # MCP server and tool handlers
│   ├── api/rest/           # REST API handlers
│   ├── db/                 # pgx pool, migration runner, sqlc queries
│   │   └── migrations/     # embedded SQL migration files (000001_…)
│   ├── embedding/          # text and code embedding backends
│   ├── memory/             # memory CRUD, hybrid retrieval, consolidation
│   ├── knowledge/          # knowledge artifact CRUD, visibility, lifecycle, synthesis
│   ├── skills/             # skill registry, install, sync
│   ├── retrieval/          # cross-layer merge and re-rank
│   ├── graph/              # entity/relation store, Apache AGE sync
│   ├── principals/         # principal and membership management
│   ├── sharing/            # sharing grant enforcement
│   └── jobs/               # background jobs scheduler
├── AGENTS.md               # ← agent rules (read first)
├── designs/                # ← architecture, design, and task documents
│   ├── DESIGN.md           #   full architecture and design decisions
│   ├── DESIGN_CODE_GRAPH.md
│   ├── DESIGN_OAUTH.md
│   ├── TASKS.md            #   current task list and status
│   └── TASKS_OAUTH.md      #   OAuth implementation task list
├── config.example.yaml
└── docker-compose.yml
```

---

## For Agents — Read This Before Implementing Anything

Before writing a single line of code, read these files **in order**:

1. **`AGENTS.md`** — mandatory rules and guard rails: TDD workflow,
   commit discipline, what you must never do. No exceptions.

2. **`designs/DESIGN.md`** — the full architecture. Understand the three-layer model,
   the database schema, the scope hierarchy, and the extension rationale
   before touching any code. Do **not** change the design without asking first.

3. **`designs/TASKS.md`** — the current task list. Find your assigned task, understand
   its scope, and update this file before every commit.

### Key constraints (summary — full rules in `AGENTS.md`)

- **TDD**: write the test before the implementation.
- **Test suite**: `go test ./...` must pass before every commit.
- **Formatter**: `gofmt -w .` before every commit.
- **Commits**: after each task; detailed message; `Co-authored-by:` trailer required.
- **designs/DESIGN.md**: do not edit without asking and explaining why.
- **No unsolicited refactoring**: mark with `TODO` and move on.
- **No heavy dependencies**: check `designs/DESIGN.md` for the approved stack first.
