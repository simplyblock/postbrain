# Code Graph — Design Notes

## Motivation

The current memory and knowledge layers treat code as opaque text. Content classified as
`"code"` gets a second embedding via a code-specialised model, but the structural meaning
of the code — what it defines, what it calls, what it imports — is invisible to the system.

The goal is to build an actual graph representation of code structure and dependencies,
stored in the existing `entities` + `relations` tables, queryable by agents and visible in
the web UI. This goes beyond semantic similarity ("find code that looks like X") and enables
structural reasoning ("what calls X?", "what breaks if Y changes?", "what does Z depend on?").

---

## What the Graph Would Contain

### Node types (`entities.entity_type`)

| Type        | Example canonical name              |
|-------------|-------------------------------------|
| `file`      | `src/auth/middleware.go`            |
| `package`   | `github.com/simplyblock/postbrain/internal/auth` |
| `function`  | `auth.VerifyToken`                  |
| `method`    | `(*TokenStore).Lookup`              |
| `type`      | `auth.Token`                        |
| `interface` | `memory.embeddingService`           |
| `struct`    | `db.Memory`                         |
| `variable`  | `auth.cookieName`                   |

### Edge types (`relations.predicate`)

| Predicate    | Meaning                                      |
|--------------|----------------------------------------------|
| `defines`    | file → defines → function/type/struct/…      |
| `imports`    | file → imports → package                    |
| `calls`      | function → calls → function                 |
| `uses`       | function → uses → type/struct               |
| `extends`    | struct/class → extends → struct/class        |
| `implements` | struct → implements → interface             |
| `exports`    | file → exports → symbol                     |

These predicates are additive — the `relations` table already stores `predicate` as a free
string, so no schema changes are required to start.

---

## Extraction Pipeline

The cleanest integration point is at memory/knowledge write time, when `source_ref` is a
`file:` reference.

```
write memory (content_kind=code, source_ref=file:src/auth/middleware.go:42)
  → Tree-sitter: parse the file
  → extract: symbols defined in this file (functions, types, interfaces, …)
  → extract: import paths
  → extract: call sites (unresolved symbol names)
  → extract: type usage (struct field types, parameter types, return types)
  → UpsertEntity for each symbol
  → UpsertRelation for each structural edge
  → (second pass) resolve call targets against known entities in the same scope
```

Because extraction is triggered by writes, the graph builds incrementally as agents store
memories. No upfront full-codebase scan is required, though a bulk-index command would be
useful for onboarding an existing repo.

---

## Tree-sitter

Tree-sitter is a fast, incremental parser with grammars for all major languages. The Go
binding is `github.com/smacker/go-tree-sitter`, which bundles pre-compiled grammars.

**What Tree-sitter reliably gives you without name resolution:**
- All symbols defined in a file (functions, methods, types, interfaces, structs, consts)
- Import paths (as written in the source — not resolved)
- Call sites (callee name, not resolved to a definition)
- Inheritance / interface relationships (by name, not resolved)

**What it cannot do on its own:**
- Resolve `foo()` to the definition of `foo` in another file
- Follow dynamic dispatch (virtual methods, interfaces, duck typing)
- Understand build tags, conditional compilation, or macros

---

## The Resolution Problem

This is the central design question. Tree-sitter gives you edges with unresolved targets
(e.g. `calls → "LookupToken"`). To close those edges you need to map the name to the entity
that defines it.

### Option A — Heuristic name matching (~70% accuracy)

After extraction, for each unresolved call target, search the entity table within the same
scope for an entity whose canonical name ends with the target symbol. Fast to implement,
no external dependencies, degrades gracefully (unresolved edges simply aren't stored).

**Pro:** buildable today, zero new infrastructure.
**Con:** false positives for common names; misses cross-scope calls.

### Option B — Language-specific tooling (~90% accuracy)

Use tools that understand the language's module system:

| Language | Tool              | Notes                                              |
|----------|-------------------|----------------------------------------------------|
| Go       | `go/packages`     | Full type-checked call graph; stdlib, no extra dep |
| Python   | `pyright` or `jedi` | LSP or library                                   |
| JS/TS    | `typescript` compiler API | Accurate but heavy                        |
| Rust     | `rust-analyzer`   | LSP                                                |

For a Go-first implementation, `go/packages` is the strongest choice: it produces a
complete, type-resolved call graph using the same compiler the project already uses.

**Pro:** near-perfect accuracy for supported languages.
**Con:** requires a Go module context; language-specific — need separate strategy per language.

### Option C — LSP-based resolution (~98% accuracy, polyglot)

Run a language server (gopls, pyright, typescript-language-server) as a sidecar.
For each call site, issue a go-to-definition request to resolve the target.

**Pro:** accurate, language-agnostic interface.
**Con:** significant operational complexity; LSP servers are long-running processes with
warm-up cost; requires the source tree to be on disk.

### Recommended starting point

Start with **Option A** for the initial implementation to validate the graph schema and UI.
Add **Option B** (Go via `go/packages`) as a high-fidelity mode once the pipeline is stable.

---

## Schema Gaps

The existing `relations` table is almost the right shape, but two gaps need addressing:

### 1. Source file tracking

When a file changes and a new memory is written, the old graph edges extracted from that
file must be invalidated. This requires knowing which relations came from which file.

**Proposed:** add `source_file TEXT` column to `relations` (nullable). On re-extraction of
a file, delete all relations where `source_file = <path>` and `scope_id = <scope>` before
inserting the fresh set.

The existing `source_memory UUID` column partially covers this — if every file extraction
creates one memory per file, `source_memory` is sufficient and no new column is needed.

### 2. Symbol-level memory chunks

Currently a memory stores an entire file (or a passage). For code graphs, finer granularity
is valuable — one memory chunk per function/method means each chunk gets its own embedding
and can be retrieved independently.

**Proposed:** a `chunks` table (or use existing `memories` with a `parent_memory_id` FK)
linking sub-function chunks to the file-level memory. Each chunk carries its own
`embedding` and `content_kind = "code"`.

This is a larger schema change and should be deferred until the base graph is working.

---

## Query Surface

Storing the graph is only useful if it can be queried. Two surfaces are relevant:

### REST / MCP

Simple traversal endpoints are sufficient for the initial implementation:

```
GET /v1/graph/callers?symbol=auth.VerifyToken&scope=project:acme/api
GET /v1/graph/callees?symbol=auth.VerifyToken&scope=project:acme/api
GET /v1/graph/deps?file=src/auth/middleware.go&scope=project:acme/api
GET /v1/graph/dependents?package=internal/auth&scope=project:acme/api
```

The existing `POST /v1/graph/query` stub (currently 501) could eventually accept a
Cypher-like or GraphQL query, but structured REST endpoints are sufficient to start.

### Web UI

The existing `/ui/graph` page shows entities and relations as tables. With code graph data
populated it becomes immediately useful. Longer term, a force-directed visualisation
(D3.js or similar) showing file → function → call edges would make dependency structures
visible at a glance.

---

## Open Questions

1. **Scope of resolution** — Phase 1 used pure suffix heuristics. Phase 4 adds
   import-aware lookup as the primary strategy, with LSP as an opt-in per-language
   enhancement. No language-specific toolchains are required dependencies.

2. **Granularity** — Phase 4 moves to function-level chunk memories. The schema
   change (`parent_memory_id`) is backward-compatible; file-level memories remain.

3. **Trigger model** — write-time extraction (for individual memories) and background
   repo indexer (for bulk/incremental) are both implemented. The chunk indexer runs
   as part of the repo sync, not at write time.

4. **Staleness** — deleted/renamed files are handled by `DeleteRelationsBySourceFile`
   during incremental sync. Entity cleanup (orphaned entities with no remaining
   relations or memories) is a future reconciliation job.

5. **Cross-scope edges** — deferred. Current graph is strictly per-scope. Cross-scope
   relation visibility should align with the sharing-grants model.

6. **LSP resolver temp-dir lifecycle** — the in-memory git clone is written to a temp
   dir before the LSP server starts. The temp dir is deleted in a `defer` even on
   crash; if the process is killed hard, OS temp-dir cleanup handles it on next boot.

7. **Chunk embedding workload** — each function chunk gets a code embedding. For large
   repos this significantly increases embedding API calls. The indexer should respect
   a configurable `REPO_CHUNK_EMBED_MAX` limit (default: embed all, skip if >N chunks).

---

## Proposed Implementation Phases

### Phase 1 — Structural extraction, heuristic resolution [COMPLETE]

- Tree-sitter extractors are implemented for 22 languages in `internal/codegraph/`:
  Go, Python, TypeScript, TSX, JavaScript, Rust, C, C++, C#, Java, Kotlin, Bash, Lua,
  PHP, Ruby, CSS, HTML, Dockerfile, HCL, Protobuf, SQL, TOML, YAML
- At write time, for `content_kind=code` memories with a `file:` source_ref: extract
  symbols and edges, UpsertEntity, UpsertRelation
- Heuristic resolution: match call targets against entities in the same scope by name suffix
- Web UI: existing `/ui/graph` page shows populated data with no changes needed

### Phase 2 — Repository attachment and bulk indexing

Project-kind scopes can have a git repository attached. The repository is the source of
truth for the code graph — bulk indexing walks the repo tree and extracts symbols/edges for
all supported files.

#### Schema (migration 000009)

Two new nullable columns on `scopes` (only meaningful for `kind = 'project'`):

```sql
ALTER TABLE scopes ADD COLUMN repo_url            TEXT;
ALTER TABLE scopes ADD COLUMN repo_default_branch TEXT NOT NULL DEFAULT 'main';
ALTER TABLE scopes ADD COLUMN last_indexed_commit  TEXT;
```

No local path column — the clone is ephemeral (see below).

#### go-git — in-process git, zero disk

Use `github.com/go-git/go-git/v5` instead of shelling out to the git CLI:
- No git binary dependency on the server
- In-memory storage: `memory.NewStorage()` — git object store lives in RAM
- In-memory worktree: `memfs.New()` from `github.com/go-git/go-git/v5/plumbing/object` — no files written to disk
- Typed API, proper error handling
- Shallow clone (`Depth: 1`) minimises data transferred

#### Full index flow

```
POST /v1/scopes/:id/repo          { url, branch? }  — attach repo + trigger initial index
POST /v1/scopes/:id/repo/sync     {}                 — re-index (fetch latest, diff, re-extract)
```

Full index:
1. `git.Clone(memory.NewStorage(), memfs.New(), {URL, Depth:1, SingleBranch, NoTags})`
2. Walk commit tree via `tree.Files().ForEach`
3. Skip files whose extension is not in `codegraph.SupportedExtensions()`
4. For each supported file: `f.Contents()` → `codegraph.Extract` → upsert entities + relations with `source_file = f.Name`
5. Set `scope.last_indexed_commit = HEAD SHA`
6. Memory is released when the clone object goes out of scope — zero disk residue

#### Incremental sync flow

1. Shallow clone HEAD (`Depth: 1`)
2. Fetch the previously indexed commit: `r.Fetch({RefSpecs: [lastSHA + ":refs/prev"], Depth: 1})`
3. `prevTree.Diff(currTree)` → list of changed/deleted files
4. For deleted files: `DELETE FROM relations WHERE source_file = $path AND scope_id = $scope`; delete entity if no remaining relations
5. For changed/added files: re-extract and upsert
6. Update `last_indexed_commit`

#### source_file tracking on relations (migration 000009)

Required for incremental sync to invalidate stale edges:

```sql
ALTER TABLE relations ADD COLUMN source_file TEXT;
CREATE INDEX relations_source_file_idx ON relations (scope_id, source_file)
    WHERE source_file IS NOT NULL;
```

#### Authentication

Repo URL encodes credentials for HTTPS: `https://token@github.com/org/repo`.
Token stored in `meta JSONB` as `{"repo_auth_token": "..."}` — never in the URL column itself.
The indexer constructs the authenticated URL at runtime.

For SSH: out of scope for Phase 2.

#### Memory limits

Configurable `REPO_INDEX_MAX_MB` (default 500). The indexer checks the pack size advertised
during the git handshake and aborts with a clear error if exceeded. Large repos can fall back
to a temp-dir shallow clone that is deleted after indexing.

### Phase 3 — Query endpoints and graph-augmented recall [COMPLETE]

- Structured traversal REST endpoints implemented: `GET /v1/graph/callers`,
  `/callees`, `/deps`, `/dependents` — each resolves a symbol and returns
  typed neighbour list with predicate, direction, confidence, source_file
- `internal/graph/traversal.go`: `ResolveSymbol`, `Callers`, `Callees`,
  `Dependencies`, `Dependents`, `NeighboursForEntity`
- `recall` MCP tool extended with `graph_depth` parameter (0=off, 1=neighbours):
  fetches graph neighbours of matched code entities, appends linked memories
  with discounted scores and `graph_context=true` flag

### Phase 4 — Chunk-level granularity and import-aware resolution

The goal is to move from file-level to function/method/class-level memories and
to close call-graph edges accurately using import context — without language-specific
toolchains.

#### 4a — Symbol range extraction (all 22 languages)

Tree-sitter nodes carry `StartPoint()` / `EndPoint()` (row, column). The
existing `Symbol` struct is extended with `StartLine`, `EndLine` fields.
Each extractor is updated to populate these from the defining CST node.

#### 4b — Chunk memories (schema migration 000010)

```sql
ALTER TABLE memories ADD COLUMN parent_memory_id UUID REFERENCES memories(id);
CREATE INDEX memories_parent_id_idx ON memories (parent_memory_id)
    WHERE parent_memory_id IS NOT NULL;
```

The repo indexer creates **one memory per extracted top-level symbol**:
- `content` = sliced source bytes `src[startByte:endByte]`
- `source_ref` = `file:<path>:<start_line>`
- `parent_memory_id` = the file-level memory for that file
- `content_kind` = `"code"`
- Gets its own embedding (code model)

The file-level memory (already created) remains as the parent. This enables
both file-level and function-level retrieval from the same index run.

#### 4c — Import-aware resolution pipeline

The current heuristic (suffix matching) is replaced by a three-stage pipeline:

```
1. Local symbol table   — exact match in file's own defined symbols
2. Import-aware lookup  — for each unresolved name, find import edges from
                          the current file entity; for each imported package/
                          module entity, search for <package>.<name> canonical
3. Suffix fallback      — existing FindEntitiesBySuffix heuristic
```

Stage 2 is entirely driven by the import edges already stored in the graph:

```
file:src/auth/service.go  --imports-->  package:internal/db
                                                 |
                                           entity: db.GetUser
                                                 ↑
call target "GetUser" resolved via import edge + canonical prefix match
```

This is language-agnostic because import edges are extracted uniformly by the
tree-sitter extractors for all 22 languages. Accuracy is substantially higher
than pure suffix matching for codebases with a complete import graph.

#### 4d — LSP-based resolution (optional, per-language)

For codebases where the import graph is insufficient (e.g. dynamic dispatch,
generated code, complex module aliasing), a `Resolver` interface allows
language-specific implementations to be plugged in:

```go
type Resolver interface {
    // Language returns the file extension this resolver handles (e.g. ".go").
    Language() string
    // Resolve maps an unresolved call target to a canonical entity name.
    // Returns "" if unresolvable.
    Resolve(ctx context.Context, file, symbol string) (canonical string, err error)
    // Close releases any resources (LSP process, temp dir).
    Close() error
}
```

Implementations start the language server on demand, keep it warm for the
duration of an index run, and shut it down when `Close()` is called. The
source tree must be on disk for LSP; the indexer writes it to a temp dir
(already in RAM from the in-memory git clone) before starting the server.

Planned implementations (order reflects value/effort ratio):

| Language   | Server                       | Notes                                  |
|------------|------------------------------|----------------------------------------|
| Go         | `gopls`                      | Highest accuracy; requires `go.mod` in tree |
| TypeScript | `typescript-language-server` | Covers TS + TSX + JS + JSX             |
| Python     | `pyright` or `pylsp`         | `pyright` preferred for accuracy       |
| Rust       | `rust-analyzer`              | Requires `Cargo.toml` in tree          |

The `Resolver` interface is registered per-language in the indexer; if no
resolver is registered for a file's language, the import-aware pipeline runs
instead. This means the system degrades gracefully and operators can opt in to
LSP support per-language without it being a hard dependency.

LSP resolver implementations are out of scope for the initial Phase 4 ship;
the interface and wiring are implemented so they can be added incrementally.

### Phase 5 — Chunk memory embedding pipeline

Chunk memories created by the repo indexer are currently stored without embeddings.
Phase 5 ensures every chunk memory gets a code-model embedding so it is retrievable
by semantic search alongside file-level memories.

#### 5a — Async embedding worker

After the indexer creates chunk memories it enqueues their IDs for embedding.
A background worker (same process, separate goroutine) drains a **priority queue**
and calls the embedding service (code model) in batches.

Priority heuristic (highest first):

`priority = in_degree(calls + imports + uses + defines) + out_degree(calls + imports + uses + defines)`

where degree is computed from the `relations` table for the chunk's linked entity.
Chunks with many incoming/outgoing links are embedded first because they are more
likely to be central to understanding and change impact.

Tie-breakers:
1. More incoming `calls` edges first (likely high fan-in hot paths/APIs)
2. Newer chunks first (`created_at DESC`)

On restart, any un-embedded memories are re-queued via a startup sweep:
`WHERE embedding_code IS NULL AND content_kind = 'code' AND is_active = true`.

Note: high degree is a strong but imperfect importance signal (for example, utility
hubs can be high-degree but low business criticality), so this prioritization should
remain configurable and observable.

#### 5b — Embedding budget caps (global + per-repo)

Use two independent configurable caps:

- `REPO_CHUNK_EMBED_MAX_GLOBAL` (default: unlimited): maximum chunk embeddings
  allowed across all repo sync jobs in a budgeting window (for example per hour
  or per day, configurable by operator policy).
- `REPO_CHUNK_EMBED_MAX_PER_REPO` (default: unlimited): maximum chunk embeddings
  allowed for a single repo in one index/sync run.

Enforcement order:
1. Apply the per-repo cap during chunk enqueue for that repo run.
2. Apply the global cap before dispatching embedding batches; once exhausted,
   remaining queued chunks are skipped or deferred to the next window.

File-level memory embedding is unaffected by these chunk caps. Splitting budgets
prevents both single-repo runaway costs and cross-repo fleet-wide spend spikes.

#### 5c — Recall integration

`RecallMemoriesByCodeVector` already queries `embedding_code`. Once chunks have
embeddings, code-mode and hybrid recall automatically surfaces function-level
results without further changes to the recall path.

---

### Phase 6 — Graph visualisation improvements

The canvas-rendered graph at `/ui/graph` shows all entities and relations for a
scope. Phase 6 makes the graph more navigable for large codebases.

#### 6a — Predicate filtering

A sidebar checkbox list lets the user toggle which predicates are visible
(`calls`, `imports`, `defines`, `uses`, `implements`, `extends`, `exports`).
Filtering is applied client-side; no new API calls required.

#### 6b — Path highlighting

Clicking a node highlights all shortest paths to/from a second selected node.
The path is computed client-side over the loaded adjacency data using BFS.
Edges and nodes not on the path are dimmed.

#### 6c — Focus / expand mode

Double-clicking a node switches to a focused view showing only that node and its
direct neighbours (depth 1). A breadcrumb trail lets the user navigate back to
the full graph or step through the expansion history.

---

### Phase 7 — Repo sync scheduling

Attached repositories are currently synced only on explicit `POST /v1/scopes/:id/repo/sync`
calls. Phase 7 adds automatic background polling.

#### 7a — Poll interval config

New nullable column `repo_sync_interval_minutes INT` on `scopes`. A value of `0`
or `NULL` disables automatic sync (default). Operators set this via the scopes
API or Web UI.

#### 7b — Scheduler goroutine

At startup, a scheduler goroutine queries all project scopes with a non-null
sync interval and fires `Syncer.Start` for each one that is due
(`now() - last_indexed_at > interval`). The check runs on a 1-minute ticker;
actual sync work is delegated to the existing `Syncer` (which prevents
concurrent syncs per scope).

#### 7c — Web UI

The repo dialog on the scopes page gains an "Auto-sync every N minutes" field
(0 = disabled). The current sync status badge is extended to show the next
scheduled sync time when auto-sync is enabled.

---

### Phase 8 — LSP resolver implementations

Concrete `LSPResolver` implementations for the four highest-value languages,
plugged into the `codegraph.Resolver` interface defined in Phase 4d.

| Language   | Server                       | Priority | Notes                                       |
|------------|------------------------------|----------|---------------------------------------------|
| Go         | `gopls`                      | 1        | Highest accuracy; requires `go.mod` in tree |
| TypeScript | `typescript-language-server` | 2        | Covers TS + TSX + JS + JSX                  |
| Python     | `pyright`                    | 3        | `pyright` preferred over `pylsp`            |
| Rust       | `rust-analyzer`              | 4        | Requires `Cargo.toml` in tree               |

Each implementation:
1. Writes the in-memory git clone to a temp dir at index start
2. Starts the language server subprocess pointed at that dir
3. Exchanges `initialize` / `initialized` handshake
4. For each call target, issues `textDocument/definition` → maps response URI+range
   back to a canonical entity name via the entity table
5. Keeps the server warm for the duration of the index run
6. Calls `Close()` which sends `shutdown`/`exit` and removes the temp dir

The implementation is opt-in: if the LSP binary is not on `PATH`, the indexer
falls back to the import-aware pipeline silently.

---

## Known Issues

### 1. Stale memories on modify / delete / rename

`indexDiff` (incremental reindex via `PrevCommit`) handles file changes as follows:

| Git change | What happens today |
|------------|--------------------|
| **Insert** | New entities, relations, and memories created — correct |
| **Modify** | Old relations deleted, file re-indexed — but **old memories persist forever** |
| **Delete** | Relations deleted — but **entities and memories persist forever** |
| **Rename** | Treated as Delete + Insert — old path's entities and memories persist |

There is no cleanup of `memories` or `entities` rows when a file is modified,
deleted, or renamed. Over time this causes unbounded accumulation of stale memory
rows, duplicated content, and orphaned entity nodes.

**Fix needed:** `indexDiff` must delete (or soft-delete) existing memories whose
`source_ref` starts with `file:<old_path>` before re-indexing a modified file,
and must delete them outright for deleted/renamed files.

### 2. No automatic reindexing

Repos are only reindexed on an explicit `POST /v1/scopes/:id/repo/sync` or the
equivalent UI button click. There is no background polling or webhook trigger.
Phase 7 of this design covers the scheduler; it has not been implemented yet.

### 3. Codegraph memories bypass the embedding pipeline

`persistFileMemory` and `persistChunkMemories` use `compat.CreateMemory` which
is a raw DB insert. It does **not** call `memory.Store`, so:

- No row is written to `embedding_index` → the reembed job never picks up these memories
- No initial embedding is computed at index time
- The memories exist in the `memories` table but will never appear in semantic search results

**Fix needed:** route codegraph memory creation through `memory.Store` (or at
minimum seed an `embedding_index` row in `pending` status so the reembed job
enrolls them).

### 4. File-level memory creation disabled by default

Because of issues 1–3 above, `persistFileMemory` (and the chunk memories derived
from it) is gated behind `IndexOptions.CodeMemory` (default `nil` → `false`) and
the config field `code_graph.code_memory` (default `false`).

Codegraph entity and relation indexing continues to work regardless of this flag.
Enable it only in environments where the stale-memory accumulation and missing
embedding enrollment are acceptable or have been fixed.

### 5. UI sync handler was using wrong principal ID (fixed)

The UI route `POST /ui/scopes/:id/repo/sync` previously read the author ID from
`auth.ContextKeyPrincipalID` in the request context. The UI middleware does not
set that key (only the REST API middleware does), so `AuthorID` was always
`uuid.Nil`. Because `persistFileMemory` returns early when `AuthorID == uuid.Nil`,
zero file memories were ever created during UI-triggered syncs.

**Fixed** in `internal/ui/handler_principals.go` by using `h.principalFromCookie(r)`
(the same helper used by every other UI handler) instead of the context key.
