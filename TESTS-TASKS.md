# Test Coverage Tasks

Incremental plan for adding unit tests across the Go codebase.
Each item is self-contained and can be picked up independently.

Legend: `[ ]` = todo · `[x]` = done

---

## Priority 1 — Pure logic, no DB required

These functions have no external dependencies and can be tested with plain
`go test` in milliseconds.

### chunking — splitter logic (`internal/chunking/chunker.go`)

- [x] `Chunk` — text shorter than `MinContentRunes` returns single-element slice
- [x] `Chunk` — text split on sentence boundaries, each chunk ≤ `maxRunes`
- [x] `Chunk` — `overlap` sentences are carried into the next chunk
- [x] `Chunk` — single unsplittable sentence hard-splits by rune count
- [x] `splitSentences` — paragraph break (`\n\n`) is a hard boundary
- [x] `splitByRunes` — prefers to break at whitespace; falls back to hard limit

File: `internal/chunking/chunker_test.go`

---

### codegraph — SSH / URL helpers (`internal/codegraph/indexer.go`)

- [ ] `isSSHURL` — SCP syntax, ssh:// scheme, HTTPS negative case
- [ ] `sshUserFromURL` — `git@github.com:…`, `ssh://user@host/…`, no-@ fallback
- [ ] `sanitizeURL` — strips user:pass from HTTPS URLs, leaves SSH URLs unchanged
- [ ] `parseSSHKey` — valid unencrypted key, valid encrypted key + passphrase, garbage input

File: `internal/codegraph/indexer_ssh_test.go`

---

### codegraph — symbol resolver (`internal/codegraph/resolve.go`)

- [ ] `Resolver.Resolve` with a fully-populated local symbol table (local hit, no DB needed)
- [ ] `Resolver.Resolve` cache: second call for the same name must not re-query
- [ ] `Resolver.Resolve` returns `(uuid.Nil, false)` for unknown symbol when pool is nil

File: `internal/codegraph/resolve_test.go`

---

### codegraph — syncer state machine (`internal/codegraph/syncer.go`)

- [ ] `NewSyncer` returns idle status for unknown scope
- [ ] `Start` transitions state to `SyncRunning`, returns `started=true`
- [ ] `Start` returns `started=false` when already running (no second goroutine)
- [ ] `Status` returns a copy (mutating the copy must not change internal state)

File: `internal/codegraph/syncer_test.go`

Note: these tests must **not** hit a real DB — stub out `IndexRepo` or pass a nil
pool and verify only the state-machine behaviour.

---

### api/rest — helper functions (`internal/api/rest/helpers.go`, `memories.go`)

- [ ] `parseScopeString` — valid `kind:externalID`, missing colon, empty string
- [ ] `paginationFromRequest` — default limit/offset, valid query params, clamp at max
- [ ] `uuidParam` — valid UUID in path, invalid string, missing param
- [ ] `entityRequestsToInput` — empty slice, single entry, multiple entries with all fields

File: `internal/api/rest/helpers_test.go`

---

### api/rest — graph response helpers (`internal/api/rest/graph.go`)

- [ ] `traversalResult` — nil input, entity with no neighbours, entity with both directions
- [ ] `scopeAndSymbol` — valid query params, missing `scope`, missing `symbol`

File: `internal/api/rest/graph_helpers_test.go`

---

### retrieval — score merging (already has tests, extend coverage)

- [ ] Verify zero-result input returns empty slice (not nil)
- [ ] Verify deduplication keeps highest score when same ID appears in multiple sources
- [ ] Min-score threshold boundary: exactly at threshold is included, just below is excluded

File: `internal/retrieval/merge_test.go` (extend existing)

---

## Priority 1.5 — No DB required; uses `embedding.FakeEmbedder`

Now that `embedding.FakeEmbedder` exists, these tests can run in the default
suite with no containers. Inject it via
`embedding.NewServiceFromEmbedders(embedding.NewFakeEmbedder(dims), nil)`.

### knowledge — store unit tests (`internal/knowledge/store.go`)

`store_test.go` already covers status flags and `ErrNotEditable`. Extend it:

- [ ] Replace the inline `fakeEmbedder` struct with `embedding.FakeEmbedder` so
  different inputs produce distinct vectors (catches accidental same-vector bugs)
- [ ] `Create` with `AutoPublish=true` sets `status=published` and non-nil `PublishedAt`
- [ ] `Create` embed error is propagated (inject a failing embedder)
- [ ] `Create` creator error is propagated
- [ ] `Update` with a nil getter result returns `ErrNotFound`
- [ ] `Update` on a draft artifact succeeds and returns updated content

File: `internal/knowledge/store_test.go` (extend existing)

---

### memory — store unit tests (`internal/memory/store.go`)

`store_test.go` already covers TTL and code-embedding path. Extend it:

- [ ] Replace the inline `mockEmbedder` struct with `embedding.FakeEmbedder` —
  single migration change, no behaviour change
- [ ] `Create` near-duplicate found → action is `"merged"`, original soft-deleted
- [ ] `Create` embed error is propagated correctly
- [ ] `Create` with large content and a real `chunkBackfill` mock verifies
  child memories are created with `parent_memory_id` set

File: `internal/memory/store_test.go` (extend existing)

---

### jobs — chunk backfill (`internal/jobs/chunk_backfill.go`)

No tests exist. The job uses `chunkBackfillStore` and `textEmbedder` interfaces,
so it is fully testable without a DB.

- [ ] `RunMemories` with zero rows is a no-op (no calls to embedder)
- [ ] `RunMemories` with one large memory creates the expected number of chunks
  (inject a fake store that returns one row; assert `createMemory` call count)
- [ ] `RunArtifacts` chunk `source_ref` has the format `artifact:<id>:chunk:<n>`
- [ ] `RunMemories` embed error on one chunk is skipped; other chunks still created
- [ ] `RunMemories` with nil embedder is a no-op (guard branch)
- [ ] Batch pagination: store returns full batch → next page fetched; partial → stop

File: `internal/jobs/chunk_backfill_test.go`

---

## Priority 2 — Requires test DB (`testcontainers`)

These tests use `testhelper.NewTestPool` to spin up a real Postgres instance.
Mark the test file with `//go:build integration` so they're skipped by default.

### graph — traversal (`internal/graph/traversal.go`)

All six public functions are currently untested.

- [ ] `ResolveSymbol` — exact canonical match, suffix fallback, not-found returns nil
- [ ] `Callers` — entity with 2 callers, entity with no callers, unknown symbol
- [ ] `Callees` — entity with 3 callees, entity with no callees
- [ ] `Dependencies` — file with imports, file with no imports
- [ ] `Dependents` — symbol with dependents, symbol with none
- [ ] `NeighboursForEntity` — mixed incoming/outgoing edges, entity with no edges

File: `internal/graph/traversal_integration_test.go`

---

### codegraph — indexer end-to-end (`internal/codegraph/indexer.go`)

- [ ] `IndexRepo` with a local bare git repo (use `git init --bare` + fixture commits)
  — verifies symbols and relations are written to DB
- [ ] Incremental diff: index, make a change, re-index — only changed file is re-processed
- [ ] `MaxBytesPerFile` cap: a file over the limit is counted in `FilesSkipped`

File: `internal/codegraph/indexer_integration_test.go`

---

### api/rest — handler unit tests with nil pool (no DB needed)

These tests pass `nil` for the pool and assert the appropriate HTTP error codes.
They don't need `testcontainers` and can run in the default test suite.

- [ ] `GET /v1/knowledge/search` — missing `q` still returns 200 (recall with empty query)
- [ ] `POST /v1/scopes/:id/repo` — missing `repo_url` returns 400
- [ ] `POST /v1/scopes/:id/repo/sync` — scope not found returns 404
- [ ] `GET /v1/scopes/:id/repo/sync` — always returns JSON (no panic with unknown scope)
- [ ] `POST /v1/memories` — malformed JSON body returns 400
- [ ] `GET /v1/memories/recall` — missing `q` returns 400

File: `internal/api/rest/knowledge_test.go`, `scopes_test.go` (extend), `memories_test.go` (extend)

---

### api/mcp — handler smoke tests (`internal/api/mcp/`)

The existing tests cover the full integration path. Add focused unit tests that
verify parameter validation without a running DB.

- [ ] `handleRecall` — missing required `query` param returns a tool error
- [ ] `handleRemember` — missing `content` param returns a tool error
- [ ] `handleForget` — invalid memory UUID in params returns a tool error
- [ ] `handlePublish` — missing `artifact_id` returns a tool error
- [ ] `handleSummarize` — missing `scope` param returns a tool error

File: `internal/api/mcp/handlers_unit_test.go`

---

### knowledge — recall pipeline (`internal/knowledge/recall.go`)

`recall_test.go` only covers score arithmetic. The full `Recall` function
requires a DB to execute the vector search, so these are integration tests.
Use `testhelper.NewFakeEmbedder(4)` for the query embedding.

- [ ] Empty query with non-nil scope runs without panic; returns empty results
- [ ] `Limit` of 0 is clamped to a sensible default (not passed as 0 to DB)
- [ ] Score merging: result from all three layers present, highest score wins
- [ ] Digest suppression: source artifact is removed when its digest is in results

File: `internal/knowledge/recall_integration_test.go`

---

### memory — consolidation edge cases (`internal/memory/consolidation.go`)

- [ ] Cluster of 1 item is not merged (no-op)
- [ ] Two identical memories produce one merged output
- [ ] `MaxClusters` limit is respected when input exceeds it

File: `internal/memory/consolidation_test.go` (extend existing)

---

## Priority 3 — UI handler coverage (`internal/ui/handler.go`)

The UI handler is tested for auth redirects. Add coverage for page rendering and
form handling. These tests use `httptest` with a nil pool (DB errors are
gracefully handled by all render functions).

- [ ] `GET /ui/knowledge` — renders without scope param (zero scope = all)
- [ ] `GET /ui/knowledge?scope=<uuid>` — selected scope is passed to template data
- [ ] `GET /ui/knowledge?q=foo&status=published` — query and status passed through
- [ ] `POST /ui/scopes/:id/repo` — missing `repo_url` shows form error (not 500)
- [ ] `POST /ui/scopes/:id/repo/sync` — fires sync and redirects (nil pool returns error gracefully)
- [ ] `GET /ui/scopes/:id/repo/sync/status` — returns JSON even for unknown scope

File: `internal/ui/handler_knowledge_test.go`, `handler_scopes_test.go`

---

## Priority 4 — Remaining language extractors

`extract_languages_test.go` already has smoke tests for most languages.
Add focused tests for edge cases that have caused regressions.

- [ ] **Go** (`extract_go.go`): generic receiver `func (r *Repo[T]) Method()` — symbol
  name and kind are correct
- [ ] **Go**: function with named return values — no duplicate symbols
- [ ] **Go**: `const` block with iota — all names extracted as `variable`
- [ ] **TypeScript**: `export default function` — extracted as function
- [ ] **TypeScript**: `class Foo extends Bar implements Baz` — both `extends` and
  `implements` edges emitted
- [ ] **Rust**: `impl Trait for Type` — `implements` edge present, method symbols correct
- [ ] **Python**: decorated function `@decorator\ndef foo()` — extracted as function
- [ ] **Java**: anonymous inner class — outer class symbol still extracted

File: `internal/codegraph/extractor_test.go` (extend existing)

---

## General guidelines

- **Use `embedding.FakeEmbedder` for all unit tests that need an embedder** —
  `embedding.NewFakeEmbedder(dims)` is deterministic, normalized, and dependency-free.
  For tests that go through `EmbeddingService`, wrap it:
  `embedding.NewServiceFromEmbedders(embedding.NewFakeEmbedder(dims), nil)`.
  Do not define local `mockEmbedder` structs; migrate existing ones to `FakeEmbedder`.

- **Use `noopXxx` base structs for interface fakes** — when a package-internal
  interface has many methods, define a `noopXxx` struct that satisfies the full
  interface with silent no-ops, then embed it in test fakes that only override the
  methods under test. See `noopLifecycleDB` in `lifecycle_test.go` as the reference
  implementation. This pattern means adding a new method to an interface only
  requires updating `noopXxx` and the real implementation — not every test fake.

- **No mocks for business logic** — prefer real types with nil/stub dependencies
  where possible; reserve mocks for I/O boundaries (DB, HTTP, git).

- **Table-driven tests** — use `[]struct{ name, input, want }` for functions
  with multiple input variants.

- **Integration test build tag** — any test requiring a live Postgres or git
  must be tagged `//go:build integration` so `go test ./...` stays fast.

- **Test file naming** — unit tests live in `*_test.go` (same package, `_test`
  suffix); integration tests in `*_integration_test.go`.

- **No test helpers without assertions** — every helper used in tests should
  call `t.Helper()` and `t.Fatal`/`t.Error` directly rather than returning
  errors for the caller to check.
