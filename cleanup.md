# Postbrain Code Cleanup Report

Generated: 2026-04-12  
Branch: `cleanup`

This document captures all findings from a comprehensive best-practices review of the postbrain Go codebase. Items are grouped by category. Each entry includes the file location, the problematic code, an explanation, and concrete fix instructions.

---

## Table of Contents

1. [Tests That Don't Test Anything Meaningful](#1-tests-that-dont-test-anything-meaningful)
2. [Raw SQL Outside sqlc](#2-raw-sql-outside-sqlc)
3. [Unused Code](#3-unused-code)
4. [Duplicate Code That Should Be Unified](#4-duplicate-code-that-should-be-unified)
5. [Overly Complicated Code Segments](#5-overly-complicated-code-segments)
6. [Code That Should Be Split (Separation of Concerns)](#6-code-that-should-be-split-separation-of-concerns)
7. [Additional Best Practices Issues](#7-additional-best-practices-issues)
8. [Additional Deep-Dive Findings (2026-04-12 Addendum)](#8-additional-deep-dive-findings-2026-04-12-addendum)

---

## 1. Tests That Don't Test Anything Meaningful

### 1.1 Compile-time signature test with no runtime assertions ✓ Done

**File:** `internal/db/schema_version_test.go:12–19`

```go
func TestSchemaVersion_Signature(t *testing.T) {
    // Compile-time signature enforcement.
    requireSchemaVersionSignature(SchemaVersion)
}

func requireSchemaVersionSignature(fn func(context.Context, *pgxpool.Pool) (uint, bool, error)) {
    _ = fn
}
```

**Issue:** This test never executes any runtime logic and makes zero assertions. The helper assigns `fn` to the blank identifier and returns. The Go compiler already enforces function signatures statically — passing a function of the wrong type would be a compile error with or without this test. This test provides a false sense of coverage without verifying any behaviour.

**Fix:** Either delete this test entirely, or replace it with a real integration test (which already exists in `internal/db/migrate_integration_test.go`) that calls `SchemaVersion` against a test database and asserts that `version > 0` and `dirty == false` after applying migrations.

---

### 1.2 Metrics test with no assertions ✓ Done

**File:** `internal/metrics/metrics_test.go:6–35`

```go
func TestMetrics_CanObserveWithoutPanic(t *testing.T) {
    t.Run("ToolDuration", func(t *testing.T) {
        ToolDuration.WithLabelValues("remember").Observe(0.001)
    })
    t.Run("EmbeddingDuration", func(t *testing.T) {
        EmbeddingDuration.WithLabelValues("ollama", "nomic-embed-text").Observe(0.05)
    })
    // ... 4 more identical sub-tests
}
```

**Issue:** No assertions whatsoever. The test name says it checks for panics, but Prometheus metric operations don't panic under normal conditions. The test adds zero value since a compile error would already catch a missing method. It does not verify that values were recorded, that label cardinality is correct, or that collectors are properly registered.

**Fix:** Either delete the test, or make it meaningful by gathering the metric values after observation and asserting they changed. For example:

```go
func TestMetrics_ToolDuration_RecordsObservation(t *testing.T) {
    before := testutil.ToFloat64(ToolDuration.WithLabelValues("remember"))
    ToolDuration.WithLabelValues("remember").Observe(0.001)
    after := testutil.ToFloat64(ToolDuration.WithLabelValues("remember"))
    if after <= before {
        t.Errorf("expected metric to increase after Observe, got before=%v after=%v", before, after)
    }
}
```

This uses `github.com/prometheus/client_golang/prometheus/testutil`, which is already an indirect dependency.

---

### 1.3 Scope fan-out test only exercises a trivial stub ✓ Done

**File:** `internal/memory/scope_test.go:29–53`

```go
func TestFanOut_NonEmpty(t *testing.T) {
    // Simulate what FanOutScopeIDs does internally after DB calls.
    ancestors := []uuid.UUID{scopeID, uuid.New(), uuid.New()}
    personal := []uuid.UUID{personalID}
    combined := deduplicateScopeIDs(append(ancestors, personal...))
    ...
}
```

**Issue:** This test manually assembles slices and passes them directly to the internal `deduplicateScopeIDs` helper, bypassing the entire `FanOutScopeIDs` function. It is effectively a test of `deduplicateScopeIDs` alone. `FanOutScopeIDs` (which does all the real work including DB calls) is not tested outside the integration test. A test named `TestFanOut_NonEmpty` should test `FanOutScopeIDs`.

**Fix:** Rename the test to `TestDeduplicateScopeIDs_RemovesDuplicates` to accurately reflect what is being tested. Add a real integration test for `FanOutScopeIDs` that uses a test database.

---

## 2. Raw SQL Outside sqlc

The project uses sqlc for query generation (`sqlc.yaml`, `internal/db/queries/*.sql`). The following files contain raw SQL strings that bypass the generated query layer. All business-logic queries should be moved to `internal/db/queries/*.sql` and regenerated via `sqlc generate`.

Infrastructure-level SQL (advisory locks in `migrate.go`, extension setup in `age_overlay.go`, dynamic DDL in `embedding_tables.go`, and Cypher-wrapping SQL in `graph/`) is legitimately beyond sqlc's scope and is excluded from these findings.

### 2.1 `internal/jobs/reembed.go` — 9+ raw SQL statements ✓ Done

**Lines:** 44–45, 68–83, 175–185, 258–262, 266–280, 287–291, 295–300, 309–314

```go
// Line 44 – raw SELECT not in sqlc
err := j.pool.QueryRow(ctx,
    `SELECT id FROM ai_models WHERE is_active=true AND model_type='embedding' AND content_type=$1`,
    contentType,
).Scan(&id)

// Line 68 – complex JOIN query inlined in production code
rows, err := j.pool.Query(ctx, `
    SELECT ei.object_type, ei.object_id, ei.retry_count, ...
    FROM embedding_index ei
    LEFT JOIN memories m ...
    ...
    WHERE ei.model_id = $1 AND ei.status = 'pending' ...
`, modelID, j.batchSize)

// Lines 258–262, 266–280 – UPDATE statements with raw SQL
j.pool.Exec(ctx, `UPDATE memories SET embedding_code=$2::vector, embedding_code_model_id=$3, updated_at=now() WHERE id=$1`, ...)
j.pool.Exec(ctx, `UPDATE memories SET embedding=$2::vector, embedding_model_id=$3, updated_at=now() WHERE id=$1`, ...)
j.pool.Exec(ctx, `UPDATE knowledge_artifacts SET embedding=$2::vector, embedding_model_id=$3, updated_at=now() WHERE id=$1`, ...)
j.pool.Exec(ctx, `UPDATE skills SET embedding=$2::vector, embedding_model_id=$3, updated_at=now() WHERE id=$1`, ...)

// Lines 287–291 – table name via string concatenation (SQL injection risk)
j.pool.QueryRow(ctx, `SELECT scope_id FROM `+map[string]string{...}[objectType]+` WHERE id=$1`, id)
```

**Fix:** Create `internal/db/queries/reembed.sql` with named queries:
- `GetActiveEmbeddingModelByContentType`
- `GetPendingEmbeddingIndexBatch`
- `GetPendingCodeEmbeddingIndexBatch`
- `UpdateMemoryTextEmbedding`
- `UpdateMemoryCodeEmbedding`
- `UpdateKnowledgeArtifactEmbedding`
- `UpdateSkillEmbedding`
- `GetMemoryScopeID`
- `GetArtifactOwnerScopeID`
- `MarkEmbeddingIndexReady`
- `MarkEmbeddingIndexFailed`

The table-name string concatenation on line 287 is a latent SQL injection risk (the map lookup prevents it at runtime, but it is fragile). Use separate sqlc queries for each object type instead.

---

### 2.2 `internal/jobs/expire.go` — raw UPDATE ✓ Done

**Lines:** 14–17

```go
tag, err := pool.Exec(ctx,
    `UPDATE memories SET is_active = false
     WHERE expires_at < now() AND is_active = true`,
)
```

**Fix:** Add to `internal/db/queries/memories.sql`:
```sql
-- name: ExpireWorkingMemories :execrows
UPDATE memories SET is_active = false
WHERE expires_at < now() AND is_active = true;
```
Then call `db.ExpireWorkingMemories(ctx, pool)`.

---

### 2.3 `internal/jobs/consolidate.go` — raw SELECT ✓ Done

**Lines:** 39–42

```go
rows, err := j.pool.Query(ctx,
    `SELECT DISTINCT scope_id FROM memories
     WHERE is_active=true AND importance < 0.7 AND access_count < 3`,
)
```

**Fix:** Add to `internal/db/queries/memories.sql`:
```sql
-- name: GetScopesWithConsolidationCandidates :many
SELECT DISTINCT scope_id FROM memories
WHERE is_active = true AND importance < 0.7 AND access_count < 3;
```

---

### 2.4 `internal/jobs/staleness.go` — 2 raw SELECTs ✓ Done

**Lines:** 78–88 (`fetchArtifactBatch`) and 224–236 (`fetchRecentMemories`)

Both are complex multi-column SELECTs that manually scan into `db.KnowledgeArtifact` and `db.Memory` structs. The manual scanning (with the embedding cast to `::text` workaround) is particularly error-prone.

**Fix:**
- Add `GetPublishedArtifactsBatch` to `internal/db/queries/knowledge.sql`
- Add `GetRecentMemoriesForScope` to `internal/db/queries/memories.sql`
- Use `pgvector` column type handling via sqlc's overrides in `sqlc.yaml` so that the `::text` cast workaround is no longer necessary.

---

### 2.5 `internal/jobs/promotion_notify.go` — raw SELECT ✓ Done

**Lines:** 33–38

```go
rows, err := j.pool.Query(ctx,
    `SELECT id, memory_id, target_scope_id, created_at
     FROM promotion_requests
     WHERE status = 'pending' AND created_at < now() - interval '24 hours'
     ORDER BY created_at`,
)
```

**Fix:** Add to `internal/db/queries/promotions.sql`:
```sql
-- name: GetStalePromotionRequests :many
SELECT id, memory_id, target_scope_id, created_at
FROM promotion_requests
WHERE status = 'pending' AND created_at < now() - interval '24 hours'
ORDER BY created_at;
```

---

### 2.6 `internal/jobs/chunk_backfill.go` — 2 raw SELECTs ✓ Done

**File:** `internal/jobs/chunk_backfill.go:44–53` and `62–72`

```go
// fetchMemoriesWithoutChunks
rows, err := p.pool.Query(ctx,
    `SELECT id, scope_id, author_id, content FROM memories
     WHERE char_length(content) > $1
       AND parent_memory_id IS NULL
       AND NOT EXISTS (SELECT 1 FROM memories c WHERE c.parent_memory_id = memories.id)
     ORDER BY created_at LIMIT $2 OFFSET $3`,
    chunking.MinContentRunes, batchSize, offset,
)

// fetchArtifactsWithoutChunks
rows, err := p.pool.Query(ctx,
    `SELECT a.id, a.owner_scope_id, a.author_id, a.content FROM knowledge_artifacts a
     WHERE char_length(a.content) > $1
       AND NOT EXISTS (SELECT 1 FROM memories m WHERE m.source_ref LIKE 'artifact:' || a.id::text || ':chunk:%')
     ORDER BY a.created_at LIMIT $2 OFFSET $3`,
    chunking.MinContentRunes, batchSize, offset,
)
```

**Fix:** Add `GetMemoriesWithoutChunks` and `GetArtifactsWithoutChunks` to the appropriate query files. Note: `chunking.MinContentRunes` is a Go constant — the threshold should be passed as a parameter `$1` as it is now, which is correct; just move the SQL to sqlc.

---

### 2.7 `internal/jobs/backfill_summaries.go` — raw SELECT and UPDATE ✓ Done

**Lines:** 32–37 and 57–59

```go
rows, err := p.pool.Query(ctx,
    `SELECT id, content FROM knowledge_artifacts WHERE summary IS NULL ORDER BY created_at LIMIT $1 OFFSET $2`,
    batchSize, offset,
)
_, err := p.pool.Exec(ctx,
    `UPDATE knowledge_artifacts SET summary=$2, updated_at=now() WHERE id=$1`,
    id, summary,
)
```

**Fix:** Add `GetUnsummarisedArtifacts` and `SetArtifactSummary` to `internal/db/queries/knowledge.sql`.

---

### 2.8 `internal/authz/resolver.go` — 7 raw queries ✓ Done

**Lines:** 48–51, 64–66, 79–100+, 138+, 171+, 188+, 223+, 276+, 285+, 301+

The entire `DBResolver.EffectivePermissions` method is built from raw SQL strings. These complex recursive CTE queries are the most security-sensitive code in the system, yet they are not managed via sqlc and have no type safety.

**Example (line 79):**
```go
err = r.pool.QueryRow(ctx, `
    SELECT EXISTS (
        WITH RECURSIVE ...
    )
`, principalID, scopeID).Scan(&ownsTargetOrAncestor)
```

**Fix:** Move all queries to `internal/db/queries/scope_grants.sql` (or a new `internal/db/queries/authz.sql`). This is the highest priority fix in this section because authorization errors are security-critical.

---

### 2.9 `internal/principals/membership.go` — 3 raw queries ✓ Done

**Lines:** 79–81, 103–105, 129–143, 167–176, 194–200

```go
// Line 79
rows, err := m.pool.Query(ctx,
    `SELECT id FROM scopes WHERE principal_id = ANY($1)`,
    allPrincipalIDs,
)
// Line 103
err := m.pool.QueryRow(ctx,
    `SELECT COALESCE((SELECT is_system_admin FROM principals WHERE id = $1), false)`,
    principalID,
).Scan(&isAdmin)
```

**Fix:** Add `GetScopesByPrincipalIDs`, `IsSystemAdmin`, `IsScopeAdmin`, `IsPrincipalAdmin`, `HasAnyAdminRole` to `internal/db/queries/principals.sql` (file already exists — add these queries there).

---

### 2.10 `internal/sharing/grants.go` — 5 raw queries ✓ Done

**Lines:** 48–52, 66–67, 77–80, 106–112, 123–129

All CRUD operations for sharing grants are raw SQL. A query file already exists at `internal/db/queries/` — these should be added there.

**Fix:** Add `CreateSharingGrant`, `RevokeSharingGrant`, `ListSharingGrantsByGrantee`, `IsMemoryAccessible`, `IsArtifactAccessible` to `internal/db/queries/` (create `sharing_grants.sql`).

---

### 2.11 `internal/memory/scope.go` — 2 raw queries ✓ Done

**Lines:** 50–52 (`filterByDepth`) and 81–83 (`personalScopeIDs`)

```go
// filterByDepth
rows, err := pool.Query(ctx,
    `SELECT id FROM scopes WHERE id = ANY($1) AND nlevel(path) <= $2`,
    ids, maxDepth,
)
// personalScopeIDs
rows, err := pool.Query(ctx,
    `SELECT id FROM scopes WHERE kind='user' AND principal_id = $1`,
    principalID,
)
```

**Fix:** Add `FilterScopesByDepth` and `GetUserScopesByPrincipal` to `internal/db/queries/scopes.sql` (file already exists).

---

### 2.12 `internal/knowledge/promote.go` — raw UPDATE ✓ Done

**Line:** 40–43

```go
_, err := p.pool.Exec(ctx,
    `UPDATE memories SET promotion_status='nominated', updated_at=now() WHERE id=$1`,
    memoryID,
)
```

**Fix:** Add `MarkMemoryNominated` to `internal/db/queries/promotions.sql` (file already exists).

---

### 2.13 `internal/db/schema_version.go` — raw SELECT on golang-migrate table ✓ Done

**Line:** 22

```go
err = conn.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&v, &d)
```

**Issue:** This queries the `schema_migrations` table that is owned by golang-migrate, not postbrain's domain model. It cannot be in sqlc without adding a model for an external tool's table. However, the query is currently not wrapped in a sqlc-compatible layer.

**Fix:** This is a legitimate infrastructure query that cannot be in sqlc. Document this exception explicitly in a comment (it is partially documented already). No change needed, but consider extracting it to a dedicated `db.ReadSchemaMigrationsVersion()` helper to make the intention clear and isolate the dependency on golang-migrate's table format.

---

### 2.14 `internal/jobs/age_backfill.go` — multiple raw queries ✓ Done

**Lines:** 27–54 (SQL constants), 143, 190 (batch fetches), 267, 275 (advisory locks)

The entity/relation batch fetching queries (lines 27–54) are business logic that should be in sqlc. The advisory lock calls are infrastructure and should stay raw.

**Fix:** Extract the four batch query constants (`ageBackfillEntityBatchSQL`, `ageBackfillRelationBatchSQL`, etc.) into sqlc query files. The advisory lock calls can remain raw.

---

## 3. Unused Code

### 3.1 Exported function `ResolveScopeByExternalID` with no callers ✓ Done

**File:** `internal/memory/scope.go:70–77`

```go
// ResolveScopeByExternalID finds a scope by kind and externalID.
func ResolveScopeByExternalID(ctx context.Context, pool *pgxpool.Pool, kind, externalID string) (*db.Scope, error) {
    s, err := db.GetScopeByExternalID(ctx, pool, kind, externalID)
    if err != nil {
        return nil, fmt.Errorf("memory: resolve scope: %w", err)
    }
    return s, nil
}
```

**Issue:** Grepping the entire codebase for `memory.ResolveScopeByExternalID` returns zero matches. This function is exported but never called. It is a thin wrapper around `db.GetScopeByExternalID` that adds no value.

**Fix:** Delete this function. All callers already use `db.GetScopeByExternalID` directly.

---

### 3.2 Internal helper `fanOutStrict` exposed only to tests via naming convention ✓ Done

**File:** `internal/memory/scope.go:115–118`

```go
// fanOutStrict is an internal helper exposed to tests: returns [scopeID].
func fanOutStrict(scopeID, _ uuid.UUID) ([]uuid.UUID, error) {
    return []uuid.UUID{scopeID}, nil
}
```

**Issue:** This function is only called from `scope_test.go`. Its body is a one-liner that is identical to the `if strictScope { return []uuid.UUID{scopeID}, nil }` branch inside `FanOutScopeIDs`. It exists only to allow a unit test to call the strict-scope branch without a database — but what is being tested is too trivial to be worth the indirection.

**Fix:** Delete `fanOutStrict`. In `scope_test.go`, replace `TestFanOut_StrictScope` with an inline call to `FanOutScopeIDs` with `strictScope=true` and mock or skip the DB portion, or simply delete the test since the branch is a one-liner already covered by integration tests.

---

## 4. Duplicate Code That Should Be Unified

### 4.1 `parseScopeString` defined in two separate packages ✓ Done

**Files:**
- `internal/api/rest/memories.go:399–410` (with comment "duplicated here to avoid a cross-package dependency")
- `internal/api/mcp/server.go:376–387`

```go
// REST copy (memories.go:401)
func parseScopeString(scope string) (string, string, error) {
    if scope == "" {
        return "", "", errString("empty scope string")
    }
    for i, c := range scope {
        if c == ':' {
            return scope[:i], scope[i+1:], nil
        }
    }
    ...
}

// MCP copy (server.go:378)
func parseScopeString(scope string) (kind, externalID string, err error) {
    if scope == "" {
        return "", "", errorString("scope: empty scope string")
    }
    idx := strings.Index(scope, ":")
    if idx < 0 {
        return "", "", errorString("scope: missing ':' separator in scope string: " + scope)
    }
    return scope[:idx], scope[idx+1:], nil
}
```

**Issue:** Two copies of the same function with slightly different error messages. The comment in `memories.go` even acknowledges the duplication. If the scope format ever changes, both copies must be updated. There are 30+ call sites across both packages.

**Fix:** Create a new internal package `internal/scopeutil` (or add to an existing utility package) with a single canonical `ParseScopeString` function. Both `internal/api/rest` and `internal/api/mcp` import it. The "cross-package dependency" concern cited in the comment is not a real impediment since neither package is imported by `scopeutil`.

---

### 4.2 Base extractor struct duplicated across 8 language extractors ✓ Done

**File:** `internal/codegraph/extract_data.go:39–865`

Eight extractor types (`cssExtractor`, `htmlExtractor`, `dockerfileExtractor`, `hclExtractor`, `protoExtractor`, `sqlExtractor`, `tomlExtractor`, `yamlExtractor`) all declare identical fields and three identical methods:

```go
// Repeated 8 times with identical bodies:
type cssExtractor struct {
    src      []byte
    filename string
    symbols  []Symbol
    edges    []Edge
}

func (e *cssExtractor) text(n *sitter.Node) string {
    return string(e.src[n.StartByte():n.EndByte()])
}

func (e *cssExtractor) addSymbol(name string, kind SymbolKind) {
    e.symbols = append(e.symbols, Symbol{Name: name, Kind: kind, File: e.filename})
}

func (e *cssExtractor) addEdge(subject, predicate, object string) {
    if subject == "" || object == "" { return }
    e.edges = append(e.edges, Edge{...})
}
```

That is 8 × ~24 lines = ~192 lines of identical boilerplate.

**Fix:** Extract into an embedded struct:

```go
type baseExtractor struct {
    src      []byte
    filename string
    symbols  []Symbol
    edges    []Edge
}

func (e *baseExtractor) text(n *sitter.Node) string { ... }
func (e *baseExtractor) addSymbol(name string, kind SymbolKind) { ... }
func (e *baseExtractor) addEdge(subject, predicate, object string) { ... }

type cssExtractor struct {
    baseExtractor
}
```

The same pattern already appears in the language-specific extractors in other files (Go, TypeScript, Rust, etc.) — confirm that all share the same `text/addSymbol/addEdge` signatures, then apply the embedding uniformly.

---

### 4.3 Scope-resolution boilerplate repeated in every MCP handler ✓ Done

Each of the ~14 MCP handler functions contains the same 6-line scope resolution block:

```go
// Appears with minor variations in: recall.go, context.go, publish.go,
// summarize.go, synthesize.go, collect.go (3x), session.go, skill_install.go,
// skill_search.go, skill_invoke.go, remember.go, graph_query.go, promote.go

kind, externalID, err := parseScopeString(scopeStr)
if err != nil {
    return mcpgo.NewToolResultError(fmt.Sprintf("...: invalid scope: %v", err)), nil
}
scope, err := db.GetScopeByExternalID(ctx, s.pool, kind, externalID)
if err != nil {
    return mcpgo.NewToolResultError(fmt.Sprintf("...: scope lookup: %v", err)), nil
}
if scope == nil {
    return mcpgo.NewToolResultError(fmt.Sprintf("...: scope '%s' not found", scopeStr)), nil
}
if err := s.authorizeRequestedScope(ctx, scope.ID); err != nil {
    return scopeAuthzToolError(ctx, "...", scope.ID, err), nil
}
scopeID = scope.ID
```

**Issue:** 14 copies of effectively the same logic. Changing the error message format, adding a new check, or changing the authorization call requires touching 14 files.

**Fix:** Add a helper method to `Server`:

```go
// resolveScopeID resolves a "kind:external_id" string to a UUID, checking authorization.
// Returns a *mcpgo.CallToolResult error result (non-nil) and uuid.Nil on failure.
func (s *Server) resolveScopeID(ctx context.Context, toolName, scopeStr string) (uuid.UUID, *mcpgo.CallToolResult) {
    kind, externalID, err := parseScopeString(scopeStr)
    if err != nil {
        return uuid.Nil, mcpgo.NewToolResultError(fmt.Sprintf("%s: invalid scope: %v", toolName, err))
    }
    scope, err := db.GetScopeByExternalID(ctx, s.pool, kind, externalID)
    if err != nil {
        return uuid.Nil, mcpgo.NewToolResultError(fmt.Sprintf("%s: scope lookup: %v", toolName, err))
    }
    if scope == nil {
        return uuid.Nil, mcpgo.NewToolResultError(fmt.Sprintf("%s: scope '%s' not found", toolName, scopeStr))
    }
    if err := s.authorizeRequestedScope(ctx, scope.ID); err != nil {
        return uuid.Nil, scopeAuthzToolError(ctx, toolName, scope.ID, err)
    }
    return scope.ID, nil
}
```

Call sites become a single two-liner:
```go
scopeID, errResult := s.resolveScopeID(ctx, "recall", scopeStr)
if errResult != nil { return errResult, nil }
```

---

### 4.4 MCP argument type-assertion boilerplate repeated in every handler ✓ Done

All MCP handlers use the same pattern for extracting typed arguments from `args map[string]any`:

```go
// Repeated with minor variations across all 14+ handlers
query, _ := args["query"].(string)
if query == "" {
    return mcpgo.NewToolResultError("...: 'query' is required"), nil
}

limit := 10
if v, ok := args["limit"].(float64); ok && v > 0 {
    limit = int(v)
}

minScore := 0.0
if v, ok := args["min_score"].(float64); ok {
    minScore = v
}
```

**Fix:** Add argument extraction helpers to `internal/api/mcp/server.go` (or a new `args.go` file in the `mcp` package):

```go
func argString(args map[string]any, key string) string {
    v, _ := args[key].(string)
    return v
}

func argIntOrDefault(args map[string]any, key string, def int) int {
    if v, ok := args[key].(float64); ok && v > 0 {
        return int(v)
    }
    return def
}

func argFloat64OrDefault(args map[string]any, key string, def float64) float64 {
    if v, ok := args[key].(float64); ok {
        return v
    }
    return def
}
```

---

### 4.5 Batch-processing loop pattern duplicated across multiple job files ✓ Done

The paginated batch-fetch-and-process loop appears in `backfill_summaries.go`, `chunk_backfill.go`, `reembed.go`, and `staleness.go`:

```go
// Appears in all 4 files with nearly identical structure:
offset := 0
total := 0
for {
    batch, err := fetch(ctx, j.batchSize, offset)
    if err != nil { return fmt.Errorf("...: fetch at offset %d: %w", kind, offset, err) }
    if len(batch) == 0 { break }
    for _, r := range batch {
        total += process(ctx, r)
    }
    slog.Info("...: batch processed", "offset", offset, ...)
    if len(batch) < j.batchSize { break }
    offset += j.batchSize
}
slog.Info("...: complete", "total", total)
```

`chunk_backfill.go` already has a `runBatch` helper method for its own two loops. The other job files repeat the same structure inline.

**Fix:** The existing `runBatch` in `chunk_backfill.go` is a good model. Evaluate whether a shared `internal/jobs/batch.go` helper can provide a generic `RunPaginatedBatch` function:

```go
func RunPaginatedBatch[T any](
    ctx context.Context,
    batchSize int,
    fetch func(ctx context.Context, batchSize, offset int) ([]T, error),
    process func(ctx context.Context, item T) error,
) (int, error)
```

---

## 5. Overly Complicated Code Segments

### 5.1 `memory.Store.Create` does 11 distinct things in one method ✓ Done

**File:** `internal/memory/store.go` (the `Create` method, approximately lines 180–300 based on the file structure)

The `Create` method performs all of the following in sequence:
1. Apply default values
2. Classify content type
3. Generate text embedding
4. Generate code embedding (conditional)
5. Calculate TTL / expiry
6. Near-duplicate detection
7. Database insert
8. Write to embedding index
9. Trigger memory chunking
10. Link named entities
11. Extract and index code graph

This is extremely difficult to test at the unit level, because any test that wants to verify duplicate detection must also mock embeddings, database, entity store, and code graph indexer.

**Fix:** Decompose into three focused methods:
- `classifyAndEmbed(ctx, m) (textVec, codeVec, error)` — steps 1–4
- `insertWithDedup(ctx, m, textVec, codeVec) (*Memory, bool, error)` — steps 5–7
- `linkMetadataAsync(ctx, m)` — steps 8–11 (can run in background goroutine)

Then `Create` becomes a thin orchestrator:
```go
func (s *Store) Create(ctx context.Context, m *Memory) (*Memory, error) {
    textVec, codeVec, err := s.classifyAndEmbed(ctx, m)
    if err != nil { return nil, err }
    created, isDuplicate, err := s.insertWithDedup(ctx, m, textVec, codeVec)
    if err != nil { return nil, err }
    if !isDuplicate {
        go s.linkMetadataAsync(context.WithoutCancel(ctx), created)
    }
    return created, nil
}
```

---

### 5.2 `ContradictionJob.fetchArtifactBatch` re-implements manual row scanning ✓ Done

**File:** `internal/jobs/staleness.go:77–122`

```go
var embText *string
err := rows.Scan(
    &a.ID, &a.KnowledgeType, &a.OwnerScopeID, ...22 fields..., &embText, ...,
)
if embText != nil {
    var v pgvector.Vector
    if err := v.Scan(*embText); err == nil {
        a.Embedding = &v
    }
}
```

The method scans 24 columns manually including the `::text` cast workaround for pgvector. This is identical in structure to the similar scanning in `fetchRecentMemories` (lines 244–270). Any change to `db.KnowledgeArtifact` or `db.Memory` struct requires updating both scan sites.

**Fix:** Once these queries are moved to sqlc (see §2.4), sqlc generates the scanning code automatically and eliminates this issue entirely. As an interim fix, extract the pgvector scanning boilerplate:

```go
func scanPgVector(text *string) *pgvector.Vector {
    if text == nil { return nil }
    var v pgvector.Vector
    if err := v.Scan(*text); err != nil { return nil }
    return &v
}
```

This helper would reduce each pgvector field from 5 lines to 1.

---

### 5.3 `resolveScopeID` uses a map for table name lookup (fragile, hard to follow) ✓ Done

**File:** `internal/jobs/reembed.go:283–292`

```go
func (j *ReembedJob) resolveScopeID(ctx context.Context, objectType string, id uuid.UUID) uuid.UUID {
    var scopeID uuid.UUID
    switch objectType {
    case "memory", "skill":
        _ = j.pool.QueryRow(ctx,
            `SELECT scope_id FROM `+map[string]string{"memory": "memories", "skill": "skills"}[objectType]+` WHERE id=$1`,
            id,
        ).Scan(&scopeID)
    case "knowledge_artifact":
        _ = j.pool.QueryRow(ctx, `SELECT owner_scope_id FROM knowledge_artifacts WHERE id=$1`, id).Scan(&scopeID)
    }
    return scopeID
}
```

**Issue:** The `map[string]string{...}[objectType]` inline lookup is used solely for table-name interpolation — which is both a SQL injection risk (mitigated only by the static switch, which is fragile) and a code smell. The `switch` already distinguishes cases; there is no reason to add a map lookup inside the first case.

**Fix:**
```go
switch objectType {
case "memory":
    _ = j.pool.QueryRow(ctx, `SELECT scope_id FROM memories WHERE id=$1`, id).Scan(&scopeID)
case "skill":
    _ = j.pool.QueryRow(ctx, `SELECT scope_id FROM skills WHERE id=$1`, id).Scan(&scopeID)
case "knowledge_artifact":
    _ = j.pool.QueryRow(ctx, `SELECT owner_scope_id FROM knowledge_artifacts WHERE id=$1`, id).Scan(&scopeID)
}
```

Or, even better, move these three queries to sqlc (see §2.1).

---

## 6. Code That Should Be Split (Separation of Concerns)

### 6.1 `internal/ui/handler.go` — 2469-line monolithic HTTP handler

**File:** `internal/ui/handler.go`

This single file contains:
- A `ServeHTTP` method with a 54-case string-switch router (lines ~188–293)
- All OAuth flow handlers (`handleOAuthAuthorize`, `handleOAuthToken`, `handleOAuthCallback`, etc.)
- All memory management handlers
- All knowledge management handlers
- All principal/scope/token management handlers
- Template rendering helpers
- Session management

The file is nearly 2500 lines. No corresponding test file exists.

**Fix:** Split by domain responsibility:
- `handler_routes.go` — The `ServeHTTP` dispatch table only
- `handler_memories.go` — Memory CRUD handlers
- `handler_knowledge.go` — Knowledge artifact handlers
- `handler_collections.go` — Collection management
- `handler_principals.go` — Principal/scope/membership management
- `handler_tokens.go` — Token management
- `handler_oauth.go` — OAuth authorization flow
- `handler_sessions.go` — Session management

Additionally, consider replacing the string-switch router with `net/http.ServeMux` (Go 1.22+ pattern routing) to eliminate the manual path parsing.

---

### 6.2 `internal/db/compat.go` — 2735-line compatibility layer with no tests

**File:** `internal/db/compat.go`

This file is 2735 lines and contains what appears to be a broad legacy compatibility / adapter layer with dozens of functions that directly execute raw SQL. It has no corresponding test file.

**Issue:** A 2735-line untested file is a maintenance and reliability liability. The file mixes:
- Simple CRUD wrappers
- Complex multi-join queries
- Business logic (e.g., event creation at line ~561)
- Migration helpers
- Data transformations

**Fix:**
1. Audit all public functions in `compat.go` and check if they have callers. Delete any that are unused.
2. For functions that remain, determine whether their SQL belongs in sqlc (it almost certainly does).
3. Split the file into focused modules: one per domain entity (memories, knowledge, scopes, tokens, events, etc.).
4. Write tests for the most critical functions.

---

### 6.3 Validation logic in REST handlers rather than request types ✓ Done

**Files:** `internal/api/rest/memories.go:42–54`, `internal/api/rest/knowledge.go:31–38`, `internal/api/rest/skills.go:31–38`, `internal/api/rest/collections.go:33–42`

Validation of required fields (`content`, `scope`, `title`) and default values (`visibility = "team"`) is scattered across handler functions:

```go
// Appears across multiple handlers:
if body.Content == "" {
    writeError(w, http.StatusBadRequest, "content is required")
    return
}
if body.Scope == "" {
    writeError(w, http.StatusBadRequest, "scope is required")
    return
}
visibility := body.Visibility
if visibility == "" {
    visibility = "team"
}
```

**Issue:** The same default value (`"team"`) is set independently in at least 3 handlers. If the default changes, all three must be updated. Error messages are inconsistent across handlers.

**Fix:** Move validation and defaults into a `Validate() error` method on the request struct, or use a validation library (the project already uses struct tags in some places). Set the default visibility in the request struct's zero value or a dedicated `ApplyDefaults()` method.

---

## 7. Additional Best Practices Issues

### 7.1 `nolint:nilerr` directive hides a legitimate concern ✓ Done

**File:** `internal/jobs/reembed.go:50`

```go
if err != nil {
    // No active model is not an error — just nothing to do.
    return nil, nil //nolint:nilerr
}
```

**Issue:** The `nolint:nilerr` directive suppresses a linter warning about returning `nil` for both the result and the error when `err != nil`. While the comment explains the intent (treating "no rows" as "nothing to do"), this pattern means the caller cannot distinguish "no model exists" from "model lookup succeeded" — both return `(nil, nil)`.

**Fix:** Explicitly check for `pgx.ErrNoRows`:
```go
if errors.Is(err, pgx.ErrNoRows) {
    return nil, nil
}
return nil, fmt.Errorf("reembed: active model lookup: %w", err)
```
This eliminates the `nolint` directive and makes the intent explicit without suppressing real errors.

---

### 7.2 `_ =` used to intentionally discard errors from error-marking helpers ✓ Done

**File:** `internal/jobs/reembed.go:102, 119, 124, 130, 217, 222, 232`

```go
_ = j.markEmbeddingFailedAttempt(ctx, r.objectType, r.id, *modelID, r.retryCount, fmt.Errorf("..."))
```

**Issue:** `markEmbeddingFailedAttempt` can fail (it runs a DB UPDATE). Discarding its error means a failed attempt won't be recorded, so the item will never be retried correctly. At minimum, failures should be logged.

**Fix:**
```go
if err := j.markEmbeddingFailedAttempt(ctx, ...); err != nil {
    slog.Error("reembed: mark failed attempt error", "object_id", r.id, "error", err)
}
```

---

### 7.3 `internal/db/conn_test.go` — tests `pgxpool.Config` setup but not actual connectivity ✓ Done

**File:** `internal/db/conn_test.go`

The test file exists but based on its name and size likely only tests configuration parsing without establishing a real connection. This means the `AfterConnect` hook that sets `search_path` (line 35 of `conn.go`) is never tested at the unit level.

**Action:** Review the test file content and ensure the `AfterConnect` hook behaviour is tested, or document why it is covered by integration tests only.

---

### 7.4 `internal/graph/pagerank.go` — raw SQL for critical graph algorithm ✓ Already fixed (error handled before this branch was cut)

**File:** `internal/graph/pagerank.go:29`

```go
pool.Exec(ctx, runPageRankSQL)
```

Where `runPageRankSQL` is a multi-line raw SQL constant defined at line 10. This runs Apache AGE's PageRank on the full graph. The result is discarded (the return value of `Exec` is ignored via `_`).

**Issue:** PageRank is a significant background operation. Silently discarding errors means failures go unnoticed.

**Fix:**
```go
if _, err := pool.Exec(ctx, runPageRankSQL); err != nil {
    return fmt.Errorf("graph: pagerank: %w", err)
}
```
Additionally, annotate or move `runPageRankSQL` to `internal/db/queries/` as a sqlc raw query once sqlc supports AGE Cypher passthrough, or at minimum document why it cannot be in sqlc.

---

### 7.5 Missing test files for substantial logic ✓ Done (authz/resolver.go ReachableScopeIDs integration tests added)

The following files contain substantial, non-trivial logic with no corresponding test file. These represent the highest-priority testing gaps:

| File | Lines | Description |
|------|-------|-------------|
| `internal/db/compat.go` | ~2735 | Legacy query adapter, critical path |
| `internal/codegraph/extract_data.go` | ~830 | 8 language extractors |
| `internal/codegraph/indexer.go` | ~405 | Code graph indexer |
| `internal/authz/resolver.go` | ~310 | Authorization resolution (security-critical) |
| `internal/ui/handler.go` | ~2469 | All UI route handlers |
| `internal/ui/auth.go` | — | Authentication handlers |
| `internal/ui/tokens.go` | — | Token management handlers |
| `internal/embedding/service_url.go` | — | Service URL resolution |
| `internal/jobs/staleness.go` | ~274 | Contradiction detection job |
| `internal/jobs/age_backfill.go` | — | AGE graph backfill |

Priority order for adding tests: `authz/resolver.go` (security) → `jobs/staleness.go` → `ui/handler.go` → codegraph extractors.

---

### 7.6 `internal/api/mcp/scopeauth.go` and REST `authorizeRequestedScope` not sharing logic ✓ Done (audit: both already call scopeauth.AuthorizeContextScope — shared path confirmed)

The MCP and REST layers each maintain their own authorization check call. In `internal/api/mcp/scopeauth.go` and in REST helpers, the authorization check calls `authz.DBResolver` via slightly different call paths. If a new authorization rule is added, it needs to be added in both layers.

**Fix:** Ensure both layers share a single `authz.Resolver` interface call site, ideally injected as a dependency into both `Server` (MCP) and the REST router. This is already partially the case but should be audited to confirm all paths go through the same resolver.

---

## 8. Additional Deep-Dive Findings (2026-04-12 Addendum)

This addendum documents additional findings discovered after the initial report, including extra static checks (`go test ./...`, `go vet ./...`, `make lint`) and targeted source scans.

### 8.1 More compile-time-only signature tests that do not validate behavior ✓ Done

**Files:**
- `internal/jobs/reembed_test.go:28–32`
- `internal/jobs/staleness_test.go:41–44`
- `internal/jobs/expire_test.go:9–13`
- `internal/jobs/consolidate_test.go:37–40`
- `internal/jobs/age_backfill_test.go:41–43`

These tests only assign function/method identifiers to blank vars (or equivalent compile-time checks) and never execute logic or assert behavior.

**Issue:** They add maintenance cost and inflate perceived coverage while testing nothing at runtime. Go compilation already enforces signatures.

**Fix:** Remove these tests, or replace each with a behavior assertion:
- constructor defaults and wiring,
- expected error handling paths,
- query construction/output semantics,
- real side effects validated via integration tests.

---

### 8.2 Integration test that effectively asserts nothing ✓ Done

**File:** `internal/ingest/extract_test.go:56–64`

```go
func TestExtractMarkitdownPPTX(t *testing.T) {
    if _, err := exec.LookPath("markitdown"); err != nil {
        t.Skip("markitdown not installed")
    }
    _, _ = ingest.Extract("slide.pptx", []byte("not a real pptx"))
}
```

**Issue:** The test ignores both return values and has no assertion. It will pass regardless of behavior as long as no panic happens.

**Fix:** Either delete the test, or make it assert a concrete contract:
- expected error class/message for invalid PPTX,
- expected subprocess invocation path,
- expected fallback behavior.

---

### 8.3 Unit test suite depends on external binary in PATH ✓ Done

**File:** `internal/ingest/extract_test.go:35–44` (`TestExtractDocx`)

`go test ./...` currently fails at `internal/ingest` with:
`ingest: markitdown not found in PATH; install with: pip install markitdown`.

**Issue:** This violates unit-test isolation and breaks deterministic CI/dev runs unless local machine state matches hidden prerequisites.

**Fix:**
1. Split tests into:
   - pure unit tests (no external binaries),
   - optional integration tests behind `//go:build integration` for markitdown subprocess behavior.
2. Add dependency injection for converter command execution so unit tests can use a fake runner.
3. Keep one integration test gated by explicit build tag/environment variable.

---

### 8.4 Raw SQL outside sqlc in model store path ✓ Done

**File:** `internal/embedding/model_store.go:39–43`, `72–77`

`DBModelStore` embeds two raw `SELECT` queries against `ai_models`.

**Issue:** This bypasses sqlc typing and duplicates query shape knowledge in application code.

**Fix:** Move to sqlc queries (for example in `internal/db/queries/models.sql`):
- `GetAIModelRuntimeConfigByID`
- `GetActiveAIModelIDByTypeAndContent`

Then call generated methods from `DBModelStore`.

---

### 8.5 Raw SQL outside sqlc in token scope restriction check ✓ Done

**File:** `internal/authz/token_resolver.go:96–104`

`TokenResolver.isScopeAllowed` uses an inline `SELECT EXISTS` with scope ancestry join logic.

**Issue:** This is authz-critical SQL embedded in service code, reducing type safety and making security logic harder to audit uniformly.

**Fix:** Move this query into sqlc (`internal/db/queries/authz.sql`) as:
- `IsRequestedScopeAllowedByTokenScopes`

Call that generated query from `TokenResolver`.

---

### 8.6 Raw SQL outside sqlc in principal store CRUD ✓ Done

**File:** `internal/principals/store.go:58–63`, `73–78`, `96`

`Update`, `UpdateProfile`, and `Delete` execute raw SQL directly.

**Issue:** Same drift/typing risk as above; SQL for principal CRUD is split between sqlc-generated and ad-hoc code.

**Fix:** Add sqlc queries:
- `UpdatePrincipalDisplayAndMeta`
- `UpdatePrincipalProfile`
- `DeletePrincipalByID`

Use generated methods in `principals.Store`.

---

### 8.7 High duplication between knowledge and skill lifecycle state machines

**Files:**
- `internal/knowledge/lifecycle.go:145–343`
- `internal/skills/lifecycle.go:81–245`

Both implementations duplicate:
- `isEffectiveAdmin` logic,
- transition guards (`draft`/`in_review`/`published`/`deprecated`),
- endorsement/autopublish mechanics,
- admin gating patterns for deprecate/republish.

**Issue:** Business rules can drift between artifact and skill lifecycles. Fixes in one lifecycle can be missed in the other.

**Fix:** Extract shared transition engine helpers into a common internal package (for example `internal/lifecyclecore`) with resource-specific adapters for DB operations and entity fields.

---

### 8.8 Overly complicated `OrchestrateRecall` control flow ✓ Done

**File:** `internal/retrieval/orchestrate.go:50–213`

`OrchestrateRecall` currently handles:
- layer orchestration,
- result mapping for three layers,
- merging/scoring truncation boundary,
- graph augmentation traversal and filtering,
- read-side write effects (`IncrementArtifactAccess`).

**Issue:** This is multiple concerns in one method, difficult to test comprehensively and reason about failure behavior.

**Fix:** Split into focused steps:
1. `collectLayerResults`
2. `mapToUnifiedResults`
3. `augmentWithGraphContext`
4. `scheduleAccessSideEffects` (or remove from this path; see next finding)

---

### 8.9 Read path mixes retrieval with asynchronous write side effects ✓ Done

**Files:**
- `internal/retrieval/orchestrate.go:103`
- `internal/memory/recall.go:276–281`

Both paths spawn goroutines with `context.Background()` and intentionally ignore DB errors while incrementing access counters.

**Issue:**
- unbounded goroutine creation under high read load,
- dropped errors hide DB pressure/failures,
- detached context ignores request cancellation/deadlines.

**Fix:**
1. Replace per-result goroutines with bounded async worker/buffer (or synchronous batched updates).
2. Pass a bounded context (`context.WithTimeout`) from request scope.
3. Record failures in logs/metrics (`access_counter_update_failures_total`).

---

### 8.10 Unused production helper only referenced by its own test ✓ Done

**File:** `internal/postbraincli/shellquote.go:5–10`

`shellSingleQuote` has no production call sites; search only finds:
- its definition,
- `internal/postbraincli/shellquote_test.go`.

**Issue:** Dead helper + dedicated tests increase maintenance burden without runtime value.

**Fix:** Remove `shellquote.go` and its test, or wire the helper into actual command construction if quoting is needed in production code.

---

### 8.11 Additional check outcomes (for traceability) ✓ Done

- `go vet ./...` completed (no vet findings; one upstream C warning from tree-sitter dependency).
- `make lint` completed (`golangci-lint` reports 0 issues).
- `go test ./...` passes (8.3 fixed; ingest uses native DOCX extraction, no external binary dependency).
- `go test -tags integration ./...` passes (27/27 packages; LSP resolver test race fixed).

---

### 8.12 `DBModelStore` mixes embedding and summary model responsibilities

**File:** `internal/embedding/model_store.go:26–90`

`DBModelStore` currently serves two distinct domains:
- embedding model lookup (`ActiveModelIDByContentType`, embedding defaults),
- generation/summary model lookup (`ActiveModelIDByTypeAndContent` with `modelType='generation'` call paths).

**Issue:** This creates a cross-domain store with mixed policy semantics. Changes in summary model behavior (fallback rules, provider config expectations, generation-specific constraints) risk unintended impact on embedding flows and vice versa.

**Fix:**
1. Split into two stores/interfaces:
   - `EmbeddingModelStore` for embedding-only concerns,
   - `SummaryModelStore` (or `GenerationModelStore`) for summary/generation concerns.
2. Keep shared low-level sqlc query methods in `internal/db`, but expose separate typed service-layer APIs.
3. Inject only the required interface into each consumer (`memory`, `retrieval`, `embedding service`) to prevent accidental coupling.
4. Add focused tests per store, including no-row behavior and model-type guard assertions.

---

### 8.13 Factory layer also mixes embedding and summary responsibilities

**Files:**
- `internal/embedding/factory.go:30–219`
- `internal/embedding/service_factory_setup.go:13–41`
- `internal/embedding/service.go:24–27` (combined resolver interface)

The current factory stack still combines:
- embedder construction (`EmbedderForModel`),
- summarizer construction (`SummarizerForModel`),
- provider/profile resolution policy,
- shared cache/state for both domains.

**Issue:** Same boundary problem as `DBModelStore`, but one layer higher. Keeping this in `internal/embedding` couples model runtime selection policy to embedding implementation details and makes generation-side evolution risky.

**Fix:**
1. Move model runtime factories to a dedicated package (for example `internal/modelruntime`).
2. Split resolver/factory interfaces by domain:
   - `EmbeddingResolver`/`EmbeddingFactory`
   - `SummaryResolver`/`SummaryFactory`
3. Keep `internal/embedding` focused on provider implementations (OpenAI/Ollama clients), not model-type routing policy.
4. Wire `EnableModelDrivenFactory` via the dedicated runtime package composition.
5. Add explicit tests for model-type boundaries (embedding cannot resolve generation-only models; summary cannot resolve embedding-only models unless explicitly configured fallback is enabled).

---

### 8.14 `codegraph.IndexRepo` mixes transport/auth, diffing, extraction, and persistence

**File:** `internal/codegraph/indexer.go:87–505`

`IndexRepo` and adjacent helpers currently combine:
- git clone/auth setup (HTTP token, SSH agent/key),
- shallow/deep clone and previous-commit fallback logic,
- tree diff strategy,
- parsing/extraction invocation,
- DB mutation side effects (delete/upsert relations, chunk writes),
- LSP resolver lifecycle.

**Issue:** Too many concerns in one module make failure handling and tests hard to isolate.

**Fix:** Split into focused packages/modules:
1. `codegraph/source` (clone/auth/fetch),
2. `codegraph/planner` (full vs diff plan),
3. `codegraph/executor` (index file actions),
4. `codegraph/writer` (DB persistence boundary).

---

### 8.15 REST router constructor is acting as both composition root and API surface owner

**File:** `internal/api/rest/router.go:24–193`

`Router` currently owns:
- dependency construction (`memory.NewStore`, `knowledge.NewStore`, etc.),
- middleware graph,
- full route registration for all domains.

**Issue:** Hard to test route wiring independently from dependency graph; difficult to evolve per-domain routing.

**Fix:** Move dependency construction out to an application/composition package and split route registration into domain modules:
- `routes_memories.go`,
- `routes_knowledge.go`,
- `routes_principals.go`,
- `routes_graph.go`, etc.

---

### 8.16 MCP server has the same mixing problem as REST router

**File:** `internal/api/mcp/server.go:26–392`

`mcp.Server` currently mixes:
- dependency construction,
- tool schema registration,
- tool authorization/middleware wrappers,
- server runtime wiring and progress notification helpers.

**Issue:** Tool schema evolution, dependency graph changes, and auth wrapper policy are tightly coupled in one file.

**Fix:** Split into:
1. MCP composition root (`server_bootstrap.go`),
2. tool registration catalogs per domain (`tools_memory.go`, `tools_knowledge.go`, ...),
3. shared wrapper/middleware utilities (`tool_wrappers.go`).

---

### 8.17 OAuth server file aggregates multiple protocol responsibilities

**File:** `internal/oauth/server.go:1–354`

One file handles metadata, authorize, token exchange, dynamic client registration, revocation, and introspection/userinfo behavior.

**Issue:** Protocol endpoint behavior changes are harder to reason about and test in isolation.

**Fix:** Split endpoint handlers by RFC surface:
- `metadata_handler.go`,
- `authorize_handler.go`,
- `token_handler.go`,
- `registration_handler.go`,
- `introspection_handler.go`,
- `revocation_handler.go`.

---

*End of Cleanup Report*
