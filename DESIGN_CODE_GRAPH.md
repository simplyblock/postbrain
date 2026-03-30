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

1. **Scope of resolution for v1** — heuristic name matching is implemented (Option A).
   The next step is whether to invest in `go/packages` for type-resolved edges; the heuristic
   approach ships a usable graph today but produces a graph agents can only partially trust
   for impact analysis.

2. **Language priority** — Go only first (fits the postbrain codebase itself), then expand?
   Or design the extraction pipeline to be language-pluggable from day one?

3. **Granularity** — file-level memories with structural edges, or function-level chunks?
   Function-level is better for retrieval but requires the chunk schema change.

4. **Trigger model** — extraction triggered by individual memory writes (incremental), a
   bulk-index command, or a background job that watches for new `source_ref=file:` memories?
   All three are useful; the write-time trigger is the minimum viable starting point.

5. **Staleness** — how do deleted or renamed files get cleaned up? A file rename creates
   a new entity but the old one lingers. Needs a reconciliation job or an explicit
   `DELETE /v1/graph/file?path=…` endpoint.

6. **Cross-scope edges** — when `project:acme/api` calls into `project:acme/shared`, should
   the edge cross scope boundaries in the graph? This intersects with the existing
   visibility and sharing-grants model.

7. **Embedding code symbols** — should individual function entities carry their own
   embedding (the function body embedded via the code model)? This enables "find functions
   similar to this one" queries beyond name matching, but doubles the embedding workload.

8. **Integration with the MCP `recall` tool** — should `recall` with `search_mode=code`
   automatically include graph-neighbour results (e.g. if `auth.VerifyToken` matches,
   also return its callees)? This is a graph-augmented RAG pattern that significantly
   improves recall completeness for code tasks.

9. **In-memory repo index crash cleanup** — for repos exceeding the in-memory size limit,
   what is the temp-dir cleanup strategy if the process crashes mid-index?

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

### Phase 3 — Query endpoints and graph-augmented recall

- Implement structured traversal REST endpoints (`callers`, `callees`, `deps`, `dependents`)
- Traversal endpoints are the prerequisite for graph-augmented recall.
- Extend `recall` to optionally include graph neighbours of matched symbols
- Web UI: visualisation of file/function dependency graph

### Phase 4 — Polyglot and chunk-level granularity

- Add language-pluggable extraction interface; add Python, TypeScript extractors
- Introduce function-level chunk memories linked to file-level parent
- LSP-based resolution as an optional high-fidelity mode
