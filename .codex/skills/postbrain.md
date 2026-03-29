# Postbrain — Persistent Memory & Knowledge for Codex

Postbrain gives you persistent memory across sessions, a shared knowledge base, and a skill registry. It is exposed as an MCP server. This skill tells you how to use it.

---

## 1. Verify the server is reachable

Before doing anything else, confirm Postbrain is available by calling `list_scopes`. If it returns an error or no scopes, stop and tell the user:

> Postbrain MCP server is not reachable. Ensure `POSTBRAIN_URL` is set (default: `http://localhost:7433`) and `POSTBRAIN_TOKEN` is valid, then retry.

Do not proceed with memory or knowledge operations if the server is unreachable.

---

## 2. Establish a scope

Every Postbrain operation requires a **scope** — a `kind:external_id` string identifying where memories and knowledge are stored.

**Scope kinds:** `company` · `department` · `project` · `user`
**Examples:** `project:acme/api` · `department:acme/engineering` · `company:acme`

**If the project's AGENTS.md (or equivalent) already declares a `POSTBRAIN_SCOPE`**, use that value directly.

**Otherwise**, call `list_scopes` and present the available scopes to the user. Ask:

> Which scope should I use for this session? (e.g. `project:acme/api`)

Do not guess or invent a scope. Wait for the user to confirm before continuing.

---

## 3. Start a session

Once you have a scope, open a session with `session_begin`. Store the returned `session_id` — you will need it for `skill_invoke` and you must pass it to `session_end` when you stop.

```
session_begin(scope="project:acme/api")
→ { "session_id": "...", "scope": "...", "started_at": "..." }
```

---

## 4. Load context

After `session_begin`, call `context` to hydrate yourself with relevant knowledge and memories for the task at hand. Pass the user's first message as `query`.

```
context(scope="project:acme/api", query="refactor the auth middleware", max_tokens=4000)
→ { "context_blocks": [...], "total_tokens": N }
```

Each block has `layer` (`knowledge` | `memory`), `type`, `content`, and optionally `full_content_available: true`. If a block has `full_content_available: true` and the summary is not enough, call `knowledge_detail` with the `id` to fetch the full artifact.

---

## 5. Tool reference

### `remember` — store a memory
Store facts, decisions, observations, and working notes.

| param | required | notes |
|---|---|---|
| `content` | yes | The memory text |
| `scope` | yes | `kind:external_id` |
| `memory_type` | — | `semantic` (default) · `episodic` · `procedural` · `working` |
| `importance` | — | 0–1, default 0.5 |
| `source_ref` | — | Provenance, e.g. `file:src/auth.go:42` |
| `entities` | — | Canonical entity names to link (array of strings) |
| `expires_in` | — | TTL in seconds; only for `working` memories |

**Always populate `entities`.** Each item is an object with `name` (canonical, lowercase) and `type`. Extract entities from the content you are storing:

| `type` | examples |
|---|---|
| `technology` | `postgresql`, `pgvector`, `redis`, `go`, `react` |
| `service` | `auth-service`, `billing-api`, `postbrain` |
| `file` | `src/auth.go`, `internal/db/compat.go` |
| `person` | `alice`, `bob` (or use `@alice` in content for auto-extraction) |
| `pr` | `pr:123` (auto-extracted from content) |
| `decision` | `use-jwt-for-sessions`, `migrate-to-postgres` |
| `concept` | `embedding`, `scope-hierarchy`, `staleness` |

Example: `[{"name":"postgresql","type":"technology"},{"name":"src/auth.go","type":"file"},{"name":"alice","type":"person"}]`

The server also auto-extracts `file:path`, `pr:NNN`, and `@mention` patterns from content, but explicit entities are preferred for precision.

**Memory type guidance:**
- `working` — temporary scratchpad (set `expires_in`); things like "currently editing X"
- `episodic` — what happened: tool calls, file edits, decisions made this session
- `procedural` — how to do something: steps, runbooks, workflows
- `semantic` — what something is: concepts, architecture, API contracts

---

### `recall` — retrieve memories and knowledge
Use before starting non-trivial work to surface what is already known.

| param | required | notes |
|---|---|---|
| `query` | yes | Natural-language or keyword query |
| `scope` | yes | `kind:external_id` |
| `layers` | — | `["memory","knowledge","skill"]` (default: all) |
| `memory_types` | — | Filter: `["semantic","episodic","procedural","working"]` |
| `search_mode` | — | `hybrid` (default) · `text` · `code` |
| `limit` | — | Max results, default 10 |
| `min_score` | — | Minimum relevance score 0–1, default 0.0 |
| `agent_type` | — | Filter skills by agent compatibility |

Returns `{ "results": [...] }`. Each result has `layer`, `id`, `score`, `content` (or `title`+`summary`), and `full_content_available`.

---

### `recall` — scope hierarchy
Recall searches the queried scope **and its ancestors**. A query against `project:acme/api` also surfaces knowledge published at `department:acme/engineering` or `company:acme` with appropriate visibility.

---

### `knowledge_detail` — fetch full artifact content
Call this when a recall result has `full_content_available: true` and the summary is insufficient.

| param | required |
|---|---|
| `artifact_id` | yes (UUID) |

---

### `forget` — deactivate or delete a memory

| param | required | notes |
|---|---|---|
| `memory_id` | yes (UUID) | |
| `hard` | — | `true` = permanent delete; default `false` (soft-delete) |

---

### `publish` — create or update a knowledge artifact
Use when a memory or finding should be preserved and shared beyond this session.

| param | required | notes |
|---|---|---|
| `title` | yes | |
| `content` | yes | Full artifact content |
| `knowledge_type` | yes | `semantic` · `episodic` · `procedural` · `reference` |
| `scope` | yes | Owner scope |
| `visibility` | — | `private` · `project` · `team` · `department` · `company` (default: `team`) |
| `summary` | — | Short summary shown in recall results |
| `auto_review` | — | `true` = move to `in_review` immediately |
| `collection_slug` | — | Add to this collection after creation |

---

### `promote` — elevate a memory into a knowledge artifact
Use when an existing memory deserves wider visibility.

| param | required |
|---|---|
| `memory_id` | yes (UUID) |
| `target_scope` | yes |
| `target_visibility` | yes |
| `proposed_title` | — |
| `collection_slug` | — |

---

### `endorse` — endorse an artifact or skill
Signals quality to other agents retrieving from this scope.

| param | required |
|---|---|
| `artifact_id` | yes (UUID) |
| `note` | — |

---

### `summarize` — consolidate memories into a semantic summary
Call at natural breakpoints (end of feature, end of day) to compress episodic memories into a semantic one.

| param | required | notes |
|---|---|---|
| `scope` | yes | |
| `topic` | — | Topic hint for clustering |
| `dry_run` | — | `true` = preview without writing |

---

### `context` — structured context bundle
Prefer this over separate `recall` calls at session start. Respects a token budget.

| param | required | notes |
|---|---|---|
| `scope` | yes | |
| `query` | — | What you are about to work on |
| `max_tokens` | — | Budget, default 4000 |

---

### `collect` — manage collections

| `action` | effect |
|---|---|
| `create_collection` | Create a named collection in a scope; requires `scope` and `name` |
| `add_to_collection` | Add an artifact; requires `artifact_id` and `collection_id` or `collection_slug`+`scope` |
| `list_collections` | List collections in a scope; requires `scope` |

---

### `synthesize_topic` — merge artifacts into a digest
Combine two or more published artifacts into a single synthesized digest artifact.

| param | required |
|---|---|
| `scope` | yes |
| `source_ids` | yes (array of UUIDs, min 2) |
| `title` | — |
| `auto_review` | — |

---

### `skill_search` — find skills in the registry

| param | required |
|---|---|
| `query` | yes |
| `scope` | — |
| `agent_type` | — |
| `limit` | — |
| `installed` | — |

---

### `skill_invoke` — expand a skill by slug
Returns the substituted skill body. Pass `session_id` for event correlation.

| param | required |
|---|---|
| `slug` | yes |
| `scope` | yes |
| `params` | — |
| `session_id` | — |
| `agent_type` | — |

---

### `skill_install` — materialise a skill into the agent command directory

| param | notes |
|---|---|
| `skill_id` or `slug` | identify the skill |
| `scope` | — |
| `agent_type` | — |
| `workdir` | target directory |

---

### `list_scopes` — list accessible scopes
No parameters. Returns all scopes visible to the current token. Use this to discover valid scope strings.

---

### `session_begin` / `session_end`

`session_begin(scope)` → `{ session_id, scope, started_at }`
`session_end(session_id)` → `{ session_id, ended_at }`

Always call `session_end` when the session terminates (in a Stop hook if available).

---

## 6. Recommended workflow

```
startup
  list_scopes                        # verify server + discover scopes
  session_begin(scope)               # open session
  context(scope, query=first_task)   # hydrate with relevant knowledge

during work
  recall(...)                        # before starting non-trivial tasks
  remember(memory_type=working, ...) # temporary notes
  remember(memory_type=episodic, ...)# decisions, edits, observations
  knowledge_detail(artifact_id)      # when summary is insufficient

wrapping up
  summarize(scope, topic)            # compress episodic → semantic
  publish(...) / promote(...)        # surface important findings
  session_end(session_id)            # close session
```

---

## 7. Environment variables

| variable | default | purpose |
|---|---|---|
| `POSTBRAIN_URL` | `http://localhost:7433` | MCP server base URL |
| `POSTBRAIN_TOKEN` | — | Bearer auth token |
| `POSTBRAIN_SCOPE` | — | Default scope (set in AGENTS.md to skip the prompt) |