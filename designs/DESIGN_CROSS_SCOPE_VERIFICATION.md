# Postbrain — Cross-Scope Verification Design

## Purpose

This document defines a dedicated verification retrieval flow for comparing
claims in one scope (for example documentation) against evidence in other
scopes (for example source code, session memory, and recent knowledge
artifacts) within an explicit time window.

It intentionally does not replace `recall`. It adds a higher-level operation
with comparison-oriented semantics, provenance guarantees, and deterministic
scope behavior.

## Problem statement

`recall` is optimized for relevance retrieval in one requested scope with
layer-specific fan-out behavior. That is correct for agent context bootstrap,
but it is not sufficient for verification workflows that need:

- explicit alternative/comparison scope targets
- deterministic per-scope evidence grouping
- strict provenance output for auditability
- time-bounded evidence (for example "since last release")

Without this, users must issue multiple manual recalls and normalize results in
client code, which causes inconsistent behavior across tools.

## Goals

- Provide one MCP call that compares evidence across multiple explicit scopes.
- Preserve current `recall` behavior and compatibility.
- Enforce existing scope authz and token downscoping for every requested scope.
- Return evidence with mandatory provenance fields.
- Support time-window filtering (`since`, `until`) as first-class inputs.
- Keep phase-1 implementation additive and low-risk.

## Non-goals

- Replacing or semantically changing `recall`.
- Automatically deciding "truth" or "resolution" between conflicting evidence.
- Introducing cross-scope write behavior.
- Building a release-management system in phase 1.

## API surface

### New MCP tool

- Name: `verify_context` (preferred) or `cross_scope_recall` (acceptable alias)
- Family: retrieval/analysis
- Permission baseline: `memories:read`

### Request shape

```json
{
  "query": "string (required)",
  "baseline_scope": "kind:external_id (required)",
  "comparison_scopes": ["kind:external_id", "kind:external_id"],
  "layers": ["memory", "knowledge", "skill"],
  "search_mode": "text|code|hybrid",
  "since": "RFC3339 timestamp (optional)",
  "until": "RFC3339 timestamp (optional)",
  "limit_per_scope": 10,
  "min_score": 0.0,
  "graph_depth": 0
}
```

### Response shape

```json
{
  "query": "...",
  "time_window": { "since": "...", "until": "..." },
  "baseline_scope": "project:docs/repo",
  "comparisons": [
    {
      "scope": "project:source/repo",
      "results": [
        {
          "layer": "memory",
          "id": "uuid",
          "score": 0.81,
          "content": "...",
          "source_ref": "file:internal/api/mcp/recall.go:120",
          "created_at": "2026-04-08T10:20:30Z",
          "updated_at": "2026-04-08T10:20:30Z",
          "scope": "project:source/repo"
        }
      ]
    }
  ],
  "baseline_results": [],
  "skipped_scopes": [
    { "scope": "project:secret/repo", "reason": "forbidden" }
  ]
}
```

## Core behavior

## Scope semantics

- `baseline_scope` and each entry in `comparison_scopes` are explicit query
  anchors.
- Baseline scope authorization failure is fatal.
- Comparison scope authorization failures are non-fatal and reported in
  `skipped_scopes`.
- Duplicate scope inputs are deduplicated in stable input order.

### Layer behavior by scope

- Memory layer:
  - default behavior in `verify_context` is strict per-scope memory retrieval.
  - no implicit ancestor/personal fan-out in phase 1 for deterministic
    comparisons.
- Knowledge layer:
  - visibility rules still apply from each scope anchor
    (`project/team/department/company/grants`).
- Skill layer:
  - scoped to the explicitly requested scope.

### Time-window behavior

- `since` and `until` apply to all layers.
- Memory window key: `memories.created_at`.
- Knowledge window key:
  - prefer `knowledge_artifacts.published_at` when non-null
  - fallback to `knowledge_artifacts.created_at`.
- Skills window key: `skills.created_at`.
- Invalid window (`since > until`) is a validation error.

### Provenance guarantees

Every returned item includes:

- `scope` (normalized `kind:external_id`)
- `layer`
- `id`
- `score`
- `source_ref` when available
- `created_at`
- `updated_at`

If fields are unavailable for an item type, they must still be present as
explicit `null` in JSON for stable clients.

## Architecture and implementation

### High-level flow

1. Parse and validate request.
2. Resolve all requested scopes to internal IDs.
3. Authorize each scope with existing scope authz path.
4. For each authorized scope:
   - execute layer retrieval with the same query and filters
   - apply time-window filtering at query layer
   - normalize results into verification result schema
5. Return grouped baseline/comparison output with skipped scopes.

### Server layer changes (`internal/api/mcp`)

- `server.go`
  - register `verify_context` tool and argument schema.
- `verify_context.go` (new)
  - implement handler with per-scope orchestration and output grouping.
- `scopeauth.go`
  - no semantic changes required; reuse `authorizeRequestedScope`.

### Retrieval orchestration changes (`internal/retrieval`)

- add a verification-specific orchestration entrypoint:
  - accepts one explicit scope anchor at a time
  - accepts `since`/`until`
  - disables implicit memory fan-out by default
- keep existing `OrchestrateRecall` unchanged for backward compatibility.

### Memory retrieval changes (`internal/memory`)

- reuse existing `RecallInput.StrictScope` behavior.
- extend recall path to accept optional time window and pass through to DB
  recall queries.

### Knowledge retrieval changes (`internal/knowledge`)

- extend `knowledge.RecallInput` with optional time window.
- apply time predicates in SQL for vector/fts/trigram recall paths so ranking is
  computed on in-window candidates only.

### DB compatibility layer changes (`internal/db/compat.go`)

- add optional time-window params in wrappers used by memory and knowledge
  recall code paths.
- keep old wrappers for existing callers until migration is complete, then
  consolidate.

## Database changes

## Required for phase 1

Schema changes are not strictly required to ship a correct first version.
Existing columns are sufficient for semantics:

- `memories.created_at`
- `knowledge_artifacts.created_at`
- `knowledge_artifacts.published_at`
- `skills.created_at`

## Required for performance and scale (phase 1.1)

Add targeted indexes to keep time-window queries bounded:

- `memories(scope_id, is_active, created_at DESC)`
- `knowledge_artifacts(owner_scope_id, status, published_at DESC)`
- `knowledge_artifacts(owner_scope_id, status, created_at DESC)` (fallback path)
- `skills(scope_id, status, created_at DESC)`

These indexes are additive and non-breaking.

## Optional phase 2 (release anchor support)

If "since last release" should be server-resolved instead of client-provided
`since`, add:

- `release_markers` table:
  - `id`, `scope_id`, `version`, `released_at`, `source_ref`, `created_by`,
    `created_at`
  - unique `(scope_id, version)`
- helper MCP tool `mark_release` (write path) or REST endpoint for CI.
- `verify_context` optional `since_release` parameter that resolves to
  `released_at`.

This phase is optional and can be deferred.

## Authorization model

- No new authz model is introduced.
- Existing token permission and scope restrictions remain authoritative.
- Tool-level permission remains read-only.
- No implicit broadening of accessible scopes from the request payload.

## Error handling

- Invalid scope format: request validation error.
- Missing `baseline_scope`: request validation error.
- Baseline unauthorized: `forbidden: scope access denied`.
- Comparison unauthorized: collected in `skipped_scopes`, request still
  succeeds.
- Invalid time window: request validation error.

## Testing strategy (strict TDD)

### Unit tests

- handler argument validation and defaults
- scope deduplication and stable output ordering
- baseline-fatal vs comparison-nonfatal authorization behavior
- invalid `since`/`until` handling
- response shape includes mandatory provenance keys

### Integration tests

- scope authz matrix for baseline and comparison scopes
- token downscoping with mixed authorized/unauthorized scope inputs
- time-window filtering across memory/knowledge layers
- deterministic strict-scope memory behavior (no ancestor/personal fan-out)

### Regression tests

- existing `recall` behavior unchanged
- existing MCP permission inventory remains valid with new tool mapping

## Rollout plan

1. Add design and task entries.
2. Implement MCP surface and retrieval plumbing behind additive code paths.
3. Add time-window SQL changes and indexes.
4. Validate with full test suite and targeted integration tests.
5. Expose to clients as opt-in new tool; do not deprecate `recall`.

## Compatibility and migration

- Backward compatible by design.
- No required client changes for existing `recall` users.
- New clients can incrementally adopt `verify_context`.

## Open questions

- Should knowledge retrieval in verification mode support a strict
  `owner_scope_only` option in addition to visibility-based lookup?
- Should comparison scope unauthorized entries be hidden completely instead of
  reported as skipped?
- Should graph augmentation be off by default in verification mode to reduce
  indirect evidence fan-out?

## Related documents

- `designs/DESIGN_API_AND_INTEGRATIONS.md`
- `designs/DESIGN_RETRIEVAL_AND_LIFECYCLE.md`
- `designs/DESIGN_DATA_MODEL.md`
- `designs/DESIGN_PERMISSIONS.md`
- `designs/TASKS.md`
