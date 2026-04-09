# Postbrain — Cross-Scope Verification Implementation Tasks

All tasks follow strict TDD:

1. write failing test(s) first (red)
2. implement minimum code to pass (green)
3. refactor while keeping tests green

See `designs/DESIGN_CROSS_SCOPE_VERIFICATION.md` for the full specification.

---

## Scope and invariants

- New MCP tool name: `cross_scope_context`
- Comparison layers in phase 1: `memory`, `knowledge`
- `recall` behavior must remain unchanged
- Authorization is per-layer and per-scope
- Baseline scope denial for a requested layer is fatal
- Comparison scope/layer denial is non-fatal and reported
- Provenance fields are mandatory in result payloads

---

## Phase 0 — Design alignment and task wiring

### 0.1 Task tracker and index wiring

- [x] Add this file to the design/task map and pair it with
      `DESIGN_CROSS_SCOPE_VERIFICATION.md`
- [x] Ensure references to old tool naming are removed from cross-scope docs

Required tests:

- [x] Doc consistency check (manual review): all references use
      `cross_scope_context`, not `verify_context`

Acceptance criteria:

- [ ] `docs/index.md` pairs
      `designs/DESIGN_CROSS_SCOPE_VERIFICATION.md` with this task file
- [ ] No contradictory tool names remain in the cross-scope design docs

---

## Phase 1 — MCP surface (handler + schema + validation)

### 1.1 Register MCP tool schema

- [x] Add `cross_scope_context` tool registration in
      `internal/api/mcp/server.go`
- [x] Define arguments:
      `query`, `baseline_scope`, `comparison_scopes`, `layers`,
      `search_mode`, `since`, `until`, `limit_per_scope`, `min_score`,
      `graph_depth`
- [x] Restrict `layers` to `memory|knowledge`

Required tests (write first):

- [x] `internal/api/mcp/server_test.go`: tool exists and schema exposes expected
      arguments
- [x] `internal/api/mcp/handlers_unit_test.go`: unknown layer is rejected

Acceptance criteria:

- [x] Tool is registered and callable
- [x] Argument validation rejects malformed `layers` and missing required fields

### 1.2 Add handler skeleton and validation

- [x] Create `internal/api/mcp/cross_scope_context.go`
- [x] Validate:
      required `query`, required `baseline_scope`, timestamp parsing,
      `since <= until`, positive `limit_per_scope`
- [x] Normalize and deduplicate scope inputs in stable order

Required tests (write first):

- [x] `internal/api/mcp/handlers_unit_test.go`:
      missing required args fail with deterministic error text
- [x] invalid scope format fails
- [x] invalid timestamp fails
- [x] `since > until` fails
- [x] duplicate scopes deduplicate while preserving first-seen order

Acceptance criteria:

- [x] Validation behavior is deterministic and fully covered by unit tests

---

## Phase 2 — Per-layer/per-scope authorization semantics

### 2.1 Implement per-layer permission checks

- [x] Enforce `memories:read` only for requested `memory` layer
- [x] Enforce `knowledge:read` only for requested `knowledge` layer
- [x] Use existing scope authz path (`authorizeRequestedScope`) with
      layer-specific required permission wiring

Required tests (write first):

- [x] `internal/api/mcp/cross_scope_context_authz_integration_test.go`:
      - `layers=["memory"]` succeeds with only `memories:read`
      - `layers=["knowledge"]` succeeds with only `knowledge:read`
      - mixed layers require both when both requested

Acceptance criteria:

- [x] Requested layers are independently authorized
- [x] No over-restriction for single-layer requests

### 2.2 Baseline fatal vs comparison non-fatal behavior

- [x] Fail request when baseline scope lacks authorization for any requested
      layer
- [x] Skip unauthorized comparison scope/layer pairs and append structured
      `skipped_scopes` entries

Required tests (write first):

- [x] `internal/api/mcp/cross_scope_context_authz_integration_test.go`:
      - baseline denied -> tool returns forbidden error
      - comparison denied -> tool succeeds and reports skip
      - mixed comparison permissions -> partial results + skips

Acceptance criteria:

- [x] Baseline denial is fatal
- [x] Comparison denial is non-fatal and observable in payload

---

## Phase 3 — Retrieval orchestration for cross-scope context

### 3.1 Add verification orchestration entrypoint

- [x] Add cross-scope orchestration function in `internal/retrieval`
- [x] Execute retrieval per authorized scope and requested layers
- [x] Keep `OrchestrateRecall` unchanged

Required tests (write first):

- [x] `internal/retrieval/orchestrate_cross_scope_test.go`:
      - per-scope grouping is stable and deterministic
      - layer filtering works (`memory` only, `knowledge` only, both)
      - no regressions in existing `OrchestrateRecall` tests

Acceptance criteria:

- [x] New path is additive
- [x] Existing recall orchestration behavior remains intact

### 3.2 Strict-scope memory behavior

- [x] Enforce strict memory retrieval (no ancestor/personal fan-out) for
      `cross_scope_context`

Required tests (write first):

- [x] `internal/memory/recall_test.go`:
      strict mode uses only anchor scope
- [x] `internal/api/mcp/cross_scope_context_integration_test.go`:
      ancestor-only memories are excluded in cross-scope mode

Acceptance criteria:

- [x] Cross-scope memory results come only from explicitly requested scopes

---

## Phase 4 — Time-window filtering

### 4.1 Memory time-window support

- [ ] Extend memory recall input with optional `since`/`until`
- [ ] Apply filters in DB query path before scoring/limit

Required tests (write first):

- [ ] `internal/memory/recall_test.go`:
      - excludes memory older than `since`
      - excludes memory newer than `until`
      - includes boundary timestamps as specified

Acceptance criteria:

- [ ] Memory layer honors request time window across all search modes

### 4.2 Knowledge time-window support

- [ ] Extend knowledge recall input with optional `since`/`until`
- [ ] Filter by `published_at` when non-null, else `created_at`
- [ ] Apply consistently to vector/fts/trigram recall paths

Required tests (write first):

- [ ] `internal/knowledge/recall_test.go`:
      published-at precedence over created-at when filtering
- [ ] `internal/knowledge/recall_integration_test.go`:
      vector/fts/trigram paths all honor time window

Acceptance criteria:

- [ ] Knowledge recall time-window logic is consistent across retrieval modes

---

## Phase 5 — Response contract and provenance guarantees

### 5.1 Normalize result schema

- [ ] Return `query`, `time_window`, `baseline_scope`, `baseline_results`,
      `scope_contexts`, `skipped_scopes`
- [ ] Include mandatory per-item provenance:
      `scope`, `layer`, `id`, `score`, `source_ref`, `created_at`, `updated_at`
- [ ] Emit explicit `null` for unavailable optional provenance fields

Required tests (write first):

- [ ] `internal/api/mcp/cross_scope_context_response_test.go`:
      payload keys and JSON shape
- [ ] ensure provenance keys always exist in serialized output

Acceptance criteria:

- [ ] Payload contract is stable and deterministic
- [ ] Provenance completeness is enforced by tests

---

## Phase 6 — Database performance indexes (phase 1.1 from design)

### 6.1 Add additive indexes for time-bounded recall

- [ ] Migration: `memories(scope_id, is_active, created_at DESC)`
- [ ] Migration: `knowledge_artifacts(owner_scope_id, status, published_at DESC)`
- [ ] Migration: `knowledge_artifacts(owner_scope_id, status, created_at DESC)`

Required tests (write first):

- [ ] `internal/db/migrations/migration_test.go`:
      new indexes exist after migration
- [ ] down migration removes new indexes cleanly

Acceptance criteria:

- [ ] Migrations are idempotent and reversible
- [ ] Existing migration chain remains valid

---

## Phase 7 — End-to-end integration and regression safety

### 7.1 End-to-end cross-scope verification flow

- [ ] Add integration test covering:
      baseline docs scope + source scope + unauthorized scope
      with mixed layers and time window

Required tests (write first):

- [ ] `internal/api/mcp/cross_scope_context_end_to_end_integration_test.go`

Acceptance criteria:

- [ ] End-to-end behavior matches design semantics for authz, grouping, and
      provenance

### 7.2 Recall non-regression

- [ ] Ensure existing `recall` API and behavior remain unchanged

Required tests:

- [ ] Existing `internal/api/mcp/recall*_test.go` suites remain green
- [ ] Existing `internal/retrieval` and `internal/memory` recall suites remain
      green

Acceptance criteria:

- [ ] No breaking changes in `recall` request/response semantics

---

## Pre-commit checklist for each implementation PR

- [ ] New/updated tests written first (red observed)
- [ ] `go test ./...` passes
- [ ] Integration tests policy applied:
      - docs-only changes: integration tests optional
      - behavior/authz/retrieval/schema changes: run targeted integration tests
      - substantial backend merge point: run full `make test-integration`
- [ ] `gofmt -w .` applied
- [ ] `make lint` passes
- [ ] `designs/TASKS.md` updated with completed iteration notes
- [ ] This task file updated with checked items and any discovered follow-ups

---

## Optional phase 8 — release anchor support (`since_release`)

### 8.1 Release marker model

- [ ] Add `release_markers` schema and query layer
- [ ] Add write path (`mark_release` MCP or REST)
- [ ] Resolve `since_release` into concrete `since`

Required tests (write first):

- [ ] model/query integration tests for create/list/resolve marker
- [ ] MCP/REST handler tests for marker write and lookup behavior
- [ ] cross-scope context tests for `since_release` precedence/validation

Acceptance criteria:

- [ ] Server can resolve release anchors deterministically
- [ ] No behavior change for clients using explicit `since`
