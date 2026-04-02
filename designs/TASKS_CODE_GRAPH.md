# Postbrain — Code Graph Implementation Tasks

All tasks follow strict TDD: failing test written first, then implementation. See `designs/DESIGN_CODE_GRAPH.md` for the full specification.

---

## Phase 1 — Structural extraction, heuristic resolution [COMPLETE]

- [x] `internal/codegraph/extractor.go` — `Symbol` struct with `Kind`, `Name`, `Canonical`, `StartLine`, `EndLine`, `StartByte`, `EndByte`; `Edge` struct with `SubjectName`, `Predicate`, `ObjectName`; `SupportedExtensions()` map
- [x] `internal/codegraph/extract_go.go` — Go extractor: functions, methods, types, interfaces, structs, imports, call sites
- [x] `internal/codegraph/extract_python.go` — Python extractor
- [x] `internal/codegraph/extract_typescript.go` — TypeScript / TSX / JavaScript extractor
- [x] `internal/codegraph/extract_rust.go` — Rust extractor
- [x] `internal/codegraph/extract_c.go` — C / C++ extractor
- [x] `internal/codegraph/extract_csharp.go` — C# extractor
- [x] `internal/codegraph/extract_java.go` — Java / Kotlin extractor
- [x] `internal/codegraph/extract_scripting.go` — Bash / Lua / PHP / Ruby extractors
- [x] `internal/codegraph/extract_data.go` — CSS / HTML / Dockerfile / HCL / Protobuf / SQL / TOML / YAML extractors
- [x] `internal/codegraph/extractor_test.go` — unit tests for all 22 language extractors
- [x] `internal/codegraph/extract_languages_test.go` — language detection tests
- [x] `internal/codegraph/resolve.go` — `LSPResolver` interface; `Resolver` struct with three-stage pipeline: (1) local symbol table, (2) import-aware lookup via graph edges, (3) suffix-heuristic fallback
- [x] `internal/codegraph/resolve_test.go` — unit tests for resolver stages
- [x] `internal/codegraph/resolve_integration_test.go` — integration tests for import-aware resolution
- [x] Write-time extraction in `internal/memory/store.go` — for `content_kind=code` memories with `file:` source_ref: extract + UpsertEntity + UpsertRelation
- [x] `internal/graph/relations.go` — `UpsertEntity`, `UpsertRelation`, `LinkMemoryToEntity`, `ListRelationsForEntity`, `ExtractEntitiesFromMemory`
- [x] `internal/graph/relations_test.go`

---

## Phase 2 — Repository attachment and bulk indexing [COMPLETE]

### Schema

- [x] `internal/db/migrations/000009_code_graph_repo.up.sql` — `ALTER TABLE scopes ADD COLUMN repo_url TEXT`, `repo_default_branch TEXT DEFAULT 'main'`, `last_indexed_commit TEXT`; `ALTER TABLE relations ADD COLUMN source_file TEXT`; `relations_source_file_idx`
- [x] `internal/db/migrations/000009_code_graph_repo.down.sql`

### Indexer

- [x] `internal/codegraph/indexer.go` — `IndexOptions` struct; `IndexRepo(ctx, pool, opts)` entry point; full-tree walk via `go-git` in-memory clone (`memory.NewStorage()` + `memfs.New()`); incremental diff via `prevTree.Diff(currTree)`; `indexFile` helper; `DeleteRelationsBySourceFile` before re-extraction of changed files; `IndexResult` with `FilesIndexed`, `SymbolsUpserted`, `ChunksCreated`, `RelationsUpserted`; `REPO_INDEX_MAX_MB` guard
- [x] `internal/codegraph/indexer.go` — SSH clone support (`ssh_key`, `ssh_key_passphrase` in `IndexOptions`); `isSSHURL`, `sshAuth`, `parseSSHKey` helpers; `golang.org/x/crypto/ssh` agent forwarding
- [x] `internal/codegraph/indexer_integration_test.go` — full-index and incremental-diff integration tests (build tag `integration`)
- [x] `internal/codegraph/indexer_ssh_test.go` — SSH URL detection tests
- [x] `internal/codegraph/syncer.go` — `Syncer` struct; `Start(pool, opts)` (prevents concurrent syncs per scope via mutex); `Status(scopeID)` returning `SyncStatus{Running, LastResult, LastError, StartedAt}`; background `run` goroutine
- [x] `internal/codegraph/syncer_test.go` — concurrent start de-duplication tests

### REST endpoints

- [x] `internal/api/rest/scopes.go` — `POST /v1/scopes/:id/repo` (attach repo + trigger initial index): accepts `{url, branch, ssh_key, ssh_key_passphrase}`; stores `repo_url`, `repo_default_branch`; starts `Syncer.Start`
- [x] `internal/api/rest/scopes.go` — `POST /v1/scopes/:id/repo/sync` (trigger incremental re-index): calls `Syncer.Start`; returns 409 if already running
- [x] `internal/api/rest/scopes.go` — `GET /v1/scopes/:id/repo/status`: returns current `SyncStatus`

### Web UI

- [x] `internal/ui/handler.go` — `handleSetScopeRepo` (`POST /ui/scopes/:id/repo`), `handleSyncScopeRepo` (`POST /ui/scopes/:id/repo/sync`), `handleSyncStatus` (`GET /ui/scopes/:id/repo/status`)
- [x] `internal/ui/web/templates/scopes.html` — repo URL + branch input form; sync button; status badge showing running/idle/error

---

## Phase 3 — Query endpoints and graph-augmented recall [COMPLETE]

### Traversal library

- [x] `internal/graph/traversal.go` — `ResolveSymbol(ctx, pool, scopeID, name)`, `Callers`, `Callees`, `Dependencies`, `Dependents`, `NeighboursForEntity`; `TraversalResult` type
- [x] `internal/graph/traversal_integration_test.go` — integration tests (build tag `integration`)

### REST endpoints

- [x] `internal/api/rest/graph.go` — `GET /v1/graph/callers?symbol=&scope_id=`, `GET /v1/graph/callees`, `GET /v1/graph/deps`, `GET /v1/graph/dependents` — each resolves symbol, returns `TraversalResult` with predicate, direction, confidence, source_file
- [x] `internal/api/rest/graph_test.go`, `graph_handlers_test.go`, `graph_helpers_test.go`

### MCP recall extension

- [x] `internal/api/mcp/recall.go` — `graph_depth` parameter (0=off, 1=neighbours): fetches graph neighbours of matched code entities via `graph.NeighboursForEntity`, appends linked memories with discounted scores and `graph_context=true` flag

---

## Phase 4 — Chunk-level granularity and import-aware resolution [COMPLETE]

### 4a — Symbol range extraction

- [x] `Symbol` struct extended with `StartLine`, `EndLine`, `StartByte`, `EndByte` (all 22 language extractors updated)

### 4b — Chunk memories schema

- [x] `internal/db/migrations/000010_chunk_memories.up.sql` — `ALTER TABLE memories ADD COLUMN parent_memory_id UUID REFERENCES memories(id) ON DELETE CASCADE`; `memories_parent_id_idx`
- [x] `internal/db/migrations/000010_chunk_memories.down.sql`
- [x] `internal/codegraph/indexer.go` — creates one chunk memory per `KindFunction / KindMethod / KindClass / KindStruct / KindInterface` symbol: `content = src[startByte:endByte]`, `source_ref = file:<path>:<line>`, `parent_memory_id = fileMemoryID`, `content_kind = "code"`

### 4c — Import-aware resolution

- [x] `internal/codegraph/resolve.go` — `Resolver.resolveViaImports` (stage 2): for each unresolved name, looks up import edges from the current file entity, then searches for `<pkg>.<name>` canonical pattern; falls back to suffix heuristic (stage 3)

### 4d — LSP resolver interface

- [x] `internal/codegraph/resolve.go` — `LSPResolver` interface defined: `Language() string`, `Resolve(ctx, file, symbol string) (canonical string, err error)`, `Close() error`
- [x] `internal/codegraph/indexer.go` — wired: `NewResolver(pool, scopeID, lsp)` accepts optional `LSPResolver`; `nil` triggers import-aware + heuristic pipeline
- [ ] No concrete `LSPResolver` implementations yet (deferred to Phase 8)

---

## Phase 5 — Chunk memory embedding pipeline [PARTIALLY COMPLETE]

### 5a — Async embedding worker with priority queue

- [ ] `internal/codegraph/embed_worker.go` — `EmbedWorker` struct; priority queue ordered by `in_degree + out_degree` from `relations` table; tie-breakers: more `calls` incoming edges first, then `created_at DESC`; drains queue in batches calling `embedding.EmbedCode`; sets `embedding_code` + `embedding_code_model_id` on memories after success; startup sweep: re-queues `WHERE embedding_code IS NULL AND content_kind='code' AND is_active=true` on launch
- [ ] `internal/codegraph/embed_worker_test.go` (TDD: failing first):
  - `TestEmbedWorker_PriorityOrder_HighDegreeFirst`
  - `TestEmbedWorker_StartupSweep_RequeuesUnembed`
  - `TestEmbedWorker_EmbedFailure_DoesNotBlockQueue`
  - `TestEmbedWorker_Stop_DrainsGracefully`

### 5b — Embedding budget caps

- [ ] `internal/config/config.go` — add `RepoChunkEmbedMaxGlobal int` and `RepoChunkEmbedMaxPerRepo int` to `Config` (or a `CodeGraphConfig` sub-struct); `SetDefault` to 0 (unlimited); document in `config.example.yaml`
- [ ] `internal/codegraph/embed_worker.go` — enforce `MaxPerRepo` cap during chunk enqueue for each repo run (tracked in `IndexResult.ChunksEnqueued`); enforce `MaxGlobal` cap before dispatching each embedding batch; skip/defer remaining once exhausted
- [ ] Tests for cap enforcement (TDD: failing first):
  - `TestEmbedWorker_PerRepoCap_StopsAtLimit`
  - `TestEmbedWorker_GlobalCap_SkipsExcess`
  - `TestEmbedWorker_FileMemoryEmbedding_UnaffectedByCaps`

### 5c — Recall integration

- [x] `RecallMemoriesByCodeVector` in DB layer already queries `embedding_code` — no further changes needed once chunks have embeddings

### Jobs wiring

- [x] `internal/jobs/chunk_backfill.go` — `ChunkBackfillJob` backfills `content`-level chunks for existing memories and knowledge artifacts (separate from repo indexer chunk memories, covers `memory.Create` and `knowledge.Create` paths)
- [x] `internal/jobs/chunk_backfill_test.go`
- [ ] `internal/jobs/scheduler.go` — start `EmbedWorker` goroutine alongside existing background jobs; wire `MaxGlobal` / `MaxPerRepo` from config

---

## Phase 6 — Graph visualisation improvements [NOT STARTED]

The `/ui/graph` page currently renders a D3 force-directed canvas graph with hover tooltips. The following enhancements are not yet implemented.

### 6a — Predicate filtering

- [ ] `internal/ui/web/templates/graph.html` — sidebar checkbox list for predicates (`calls`, `imports`, `defines`, `uses`, `implements`, `extends`, `exports`); client-side filtering toggles link visibility without reloading data; checkboxes default to all-on
- [ ] Tests (`internal/ui/handler_graph_test.go`):
  - `TestHandleGraph_RendersPredicateCheckboxes` (checks template output)

### 6b — Path highlighting

- [ ] `internal/ui/web/templates/graph.html` — clicking a second node after selecting a first triggers BFS over the loaded adjacency data; edges and nodes not on the shortest path are dimmed; clicking on empty space clears selection
- [ ] No new server-side changes required

### 6c — Focus / expand mode

- [ ] `internal/ui/web/templates/graph.html` — double-click (`dblclick`) on a node switches to focused view (node + depth-1 neighbours only); breadcrumb trail (`Full graph > NodeA > NodeB`) allows navigation back; clicking "Full graph" restores all nodes

---

## Phase 7 — Repo sync scheduling [NOT STARTED]

### 7a — Schema

- [ ] `internal/db/migrations/000012_repo_sync_schedule.up.sql` — `ALTER TABLE scopes ADD COLUMN repo_sync_interval_minutes INT`; default `NULL` (disabled)
- [ ] `internal/db/migrations/000012_repo_sync_schedule.down.sql`

### 7b — Scheduler goroutine

- [ ] `internal/codegraph/repo_scheduler.go` — `RepoScheduler` struct; 1-minute ticker goroutine; queries all project scopes with `repo_sync_interval_minutes IS NOT NULL AND repo_url IS NOT NULL`; fires `Syncer.Start` for scopes where `now() - last_indexed_at > interval`; prevents duplicate concurrent syncs (Syncer already guards this)
- [ ] `internal/codegraph/repo_scheduler_test.go` (TDD: failing first):
  - `TestRepoScheduler_DueScope_Triggers`
  - `TestRepoScheduler_NotDueScope_Skips`
  - `TestRepoScheduler_NullInterval_NeverTriggers`
  - `TestRepoScheduler_ConcurrentSyncRunning_DoesNotDoubleStart`
- [ ] `cmd/postbrain/main.go` — start `RepoScheduler` alongside existing background jobs in `runServe`

### 7c — REST + Web UI

- [ ] `internal/api/rest/scopes.go` — `PATCH /v1/scopes/:id/repo` — update `repo_sync_interval_minutes` (0 or null = disabled)
- [ ] `internal/ui/handler.go` — update `handleSetScopeRepo` / scopes form to include "Auto-sync every N minutes" field (0 = disabled)
- [ ] `internal/ui/web/templates/scopes.html` — "Auto-sync every N minutes" input in repo dialog; status badge extended to show next scheduled sync time when interval is set

---

## Phase 8 — LSP resolver implementations [NOT STARTED]

### Interface wiring (prerequisite, already done in Phase 4d)

- [x] `LSPResolver` interface defined in `internal/codegraph/resolve.go`
- [x] `Resolver` wired to accept an optional `LSPResolver`

### 8a — Go (gopls)

- [ ] `internal/codegraph/lsp_go.go` — `GoLSPResolver` implementing `LSPResolver`:
  1. Write in-memory git clone to temp dir at index start
  2. Start `gopls` subprocess via `os/exec`
  3. Exchange `initialize` / `initialized` JSON-RPC handshake
  4. For each call target: issue `textDocument/definition` request; map response URI+range back to canonical entity name via entity table
  5. Keep server warm for duration of index run
  6. `Close()` sends `shutdown` / `exit` and removes temp dir
- [ ] `internal/codegraph/lsp_go_test.go` (TDD: failing first; skip if `gopls` not on PATH):
  - `TestGoLSPResolver_ResolvesKnownFunction`
  - `TestGoLSPResolver_UnresolvableTarget_ReturnsEmpty`
  - `TestGoLSPResolver_Close_RemovesTempDir`

### 8b — TypeScript (typescript-language-server)

- [ ] `internal/codegraph/lsp_typescript.go` — `TypeScriptLSPResolver`; covers `.ts`, `.tsx`, `.js`, `.jsx`; skip if `typescript-language-server` not on PATH
- [ ] `internal/codegraph/lsp_typescript_test.go`

### 8c — Python (pyright)

- [ ] `internal/codegraph/lsp_python.go` — `PythonLSPResolver`; uses `pyright`; skip if not on PATH
- [ ] `internal/codegraph/lsp_python_test.go`

### 8d — Rust (rust-analyzer)

- [ ] `internal/codegraph/lsp_rust.go` — `RustLSPResolver`; requires `Cargo.toml` in tree; skip if `rust-analyzer` not on PATH
- [ ] `internal/codegraph/lsp_rust_test.go`

### Registry

- [ ] `internal/codegraph/lsp_registry.go` — `NewLSPRegistry() map[string]LSPResolver` — auto-detects available servers via `exec.LookPath`; returns only those found; used in `IndexRepo` when `opts.EnableLSP = true`
- [ ] `internal/codegraph/lsp_registry_test.go`:
  - `TestNewLSPRegistry_MissingBinary_NotIncluded`
  - `TestNewLSPRegistry_PresentBinary_Included` (requires mocked PATH)

---

## Summary

| Phase | Status |
|-------|--------|
| 1 — Structural extraction, heuristic resolution | **Complete** |
| 2 — Repository attachment and bulk indexing | **Complete** |
| 3 — Query endpoints and graph-augmented recall | **Complete** |
| 4 — Chunk-level granularity and import-aware resolution | **Complete** (LSP interface only; no concrete implementations) |
| 5 — Chunk memory embedding pipeline | **Partial** — chunk backfill job exists; async worker with priority queue + budget caps missing |
| 6 — Graph visualisation improvements | **Not started** |
| 7 — Repo sync scheduling | **Not started** |
| 8 — LSP resolver implementations | **Not started** |
