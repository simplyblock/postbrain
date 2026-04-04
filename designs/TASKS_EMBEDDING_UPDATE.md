# TASKS — Embedding Architecture Update

## Goal

Move from fixed inline embedding columns to a model-scoped embedding architecture that supports:
- one embedding storage table per embedding model/dimension contract,
- multiple embedding providers/services in parallel,
- safe model registration and activation,
- re-embedding across providers without breaking recall of existing embeddings.

---

## Phase 1: Schema & Contracts

- [ ] Define and approve the storage contract:
  - One physical embedding table per registered embedding model.
  - Table schema uses fixed `vector(N)` where `N = embedding_models.dimensions`.
  - One model row maps to exactly one provider/service/model slug and one dimension contract.

- [ ] Extend `embedding_models` schema:
  - Add table mapping fields (for example `table_name`, `is_ready`, `created_at`).
  - Add provider/runtime fields (for example `provider`, `service_url`, `provider_model`).
  - Add constraints/indexes for uniqueness and safe lifecycle transitions.

- [ ] Define naming and validation rules for per-model tables:
  - deterministic/safe table name generation,
  - SQL identifier sanitization,
  - uniqueness guarantees.

---

## Phase 2: Migrations & Bootstrap

- [ ] Add non-breaking migration(s) to introduce new metadata columns and helper SQL primitives for dynamic table creation.

- [ ] Add migration/bootstrap logic to register current active models into the new schema and create their backing embedding tables.

- [ ] Keep legacy inline columns intact during transition.

- [ ] Add verification SQL checks:
  - table exists for every ready model,
  - dimensions in table type match `embedding_models.dimensions`.

---

## Phase 3: Embedding Storage Abstraction

- [ ] Introduce a dedicated embedding repository layer in `internal/db` that routes by model mapping.

- [ ] Implement core operations:
  - upsert embedding for `(object_type, object_id, model_id)`,
  - fetch embedding by object + model,
  - ANN query per scope/object type/model table.

- [ ] Enforce strict dimension checks at repository boundary.

---

## Phase 4: Multi-Provider Embedder Factory

- [ ] Replace single global backend assumption with model-driven provider resolution:
  - `EmbedderForModel(modelID)` builds provider client from model config.

- [ ] Support at least:
  - `provider=ollama` with per-model `service_url`,
  - `provider=openai` with per-model `service_url` + optional auth.

- [ ] Ensure write paths use active model by content type, but embedder instantiation is model-specific.

---

## Phase 5: Model Registration & CLI

- [ ] Implement service/API path for model registration:
  - insert model metadata,
  - create table/indexes for that model,
  - persist table mapping,
  - mark model ready only after successful provisioning.

- [ ] Add new CLI command(s):
  - `embedding-model register` (provider, service_url, provider_model, dimensions, content_type, optional activate),
  - `embedding-model activate`,
  - `embedding-model list`.

- [ ] Add idempotency and failure cleanup behavior for partially created resources.

---

## Phase 6: Query/Recall Refactor

- [ ] Update memory/knowledge/skills/entity recall to query embeddings via repository instead of inline vector columns.

- [ ] Default retrieval strategy:
  - query active model for the content type.

- [ ] Define fallback behavior when active-model embedding is missing:
  - fallback to prior active model and/or trigger async backfill.

- [ ] Keep score handling explicit; do not blend incompatible model spaces without normalization rules.

---

## Phase 7: Re-Embedding Pipeline

- [ ] Refactor re-embed job to target a specific model ID.

- [ ] Re-embed flow:
  - load raw content,
  - embed via target model provider config,
  - write embedding to target model table,
  - update object->model linkage state as required.

- [ ] Ensure old embeddings remain queryable during migration windows.

---

## Phase 8: Compatibility & Cutover

- [ ] Phase A (dual-write): write both legacy columns and new model tables.

- [ ] Phase B (dual-read): read from new tables first, fallback legacy if missing.

- [ ] Phase C (new-only): remove legacy read path after validation.

- [ ] Phase D (cleanup migration): drop legacy embedding columns/indexes only after explicit sign-off.

---

## Phase 9: Testing Strategy (TDD-first)

- [ ] Unit tests:
  - table-name generation and sanitization,
  - dimension contract enforcement,
  - provider factory resolution,
  - registration lifecycle validation.

- [ ] Integration tests:
  - register model -> table/index created,
  - write/read/ANN queries through model tables,
  - multi-provider coexistence,
  - re-embed to new model while old model remains queryable.

- [ ] Regression tests:
  - scope-locked recall correctness,
  - MCP/REST behavior unchanged from caller perspective.

---

## Phase 10: Ops & Documentation

- [ ] Update user docs:
  - model registration workflow,
  - provider/service configuration per model,
  - activation and rollback procedures,
  - re-embedding runbook.

- [ ] Add operational checks:
  - index/table health verification,
  - model readiness dashboards/metrics,
  - migration rollback notes.

---

## Open Design Decisions (Need Approval)

- [x] Physical table naming convention:
  - `embeddings_model_<uuid_no_dashes>`

- [x] Object linkage strategy:
  - central polymorphic embedding metadata table + per-model vector tables.

- [x] Fallback retrieval policy:
  - fallback only when embedding for the active/default model is missing.
  - no cross-model score blending in a single ranking pass.

- [x] Timeline for legacy column deprecation and cleanup migration:
  - fast/aggressive cutover (option 1).

---

## Ordered Execution Plan (TDD-Gated)

This section defines the precise implementation order.  
Every step follows strict Red -> Green -> Refactor before moving to the next step.

### TDD Rules (Mandatory for Every Step)

- [ ] Write failing test(s) first (`RED`) for the exact behavior in the step.
- [ ] Confirm failure is real (test fails for expected reason).
- [ ] Implement the minimum code to pass (`GREEN`).
- [ ] Refactor while keeping tests green (`REFACTOR`).
- [ ] Run step-level tests, then full `go test ./...`, `gofmt -w .`, and `make lint`.
- [ ] Update this tasks file status before commit.

### Step 0: Lock Design Decisions

- [ ] Finalize table naming strategy (`UUID`-based vs `slug/version`-based).
- [ ] Finalize object linkage strategy (`central metadata table` vs `fully per-model tables`).
- [ ] Finalize fallback retrieval policy for missing active-model embeddings.
- [ ] Freeze compatibility timeline for legacy columns.

### Step 1: Schema Preparation

- [ ] RED: migration tests for new `embedding_models` metadata columns and constraints.
- [ ] GREEN: add migration(s) for provider/table metadata fields.
- [ ] REFACTOR: tighten constraints/indexes and migration comments.

### Step 2: Dynamic Table Provisioning Primitives

- [ ] RED: tests for safe table-name generation and SQL identifier validation.
- [ ] GREEN: implement helper(s) to create per-model embedding table + ANN index.
- [ ] REFACTOR: isolate SQL generation and reuse in registration flow.

### Step 3: Model Registration Backend Flow

- [ ] RED: tests for registration transaction behavior (insert model, create table/index, map table, mark ready).
- [ ] GREEN: implement registration orchestration with failure cleanup/idempotency.
- [ ] REFACTOR: split validation/provisioning/persistence responsibilities.

### Step 4: CLI Command for Model Registration

- [ ] RED: command tests for argument validation and success/failure output.
- [ ] GREEN: add `embedding-model register` command with required provider/service/model/dim args.
- [ ] REFACTOR: share parsing/validation with other embedding-model commands.

### Step 5: Multi-Provider Embedder Factory

- [ ] RED: tests for `EmbedderForModel(modelID)` provider resolution and service URL/auth behavior.
- [ ] GREEN: implement factory for `ollama` and `openai`.
- [ ] REFACTOR: unify client construction and error messaging.

### Step 6: Embedding Repository Layer

- [ ] RED: tests for per-model-table upsert/get/query routing and dimension enforcement.
- [ ] GREEN: add repository API in `internal/db` for model-scoped embedding operations.
- [ ] REFACTOR: centralize model/table lookup caching and retry-safe DB access.

### Step 7: Write Path Integration (Dual-Write Start)

- [ ] RED: tests ensuring memory/knowledge/skills/entity writes store embeddings in new model table.
- [ ] GREEN: integrate repository into write paths, keep legacy column writes enabled.
- [ ] REFACTOR: reduce duplication across memory/knowledge/synthesis write paths.

### Step 8: Read/Recall Path Integration (Dual-Read)

- [ ] RED: tests ensuring recall prefers new model tables and falls back per approved policy.
- [ ] GREEN: route ANN retrieval through new repository.
- [ ] REFACTOR: normalize scoring interfaces and keep scope auth semantics unchanged.

### Step 9: Re-Embed Pipeline Refactor

- [ ] RED: tests for `reembed --target-model` behavior with mixed providers.
- [ ] GREEN: refactor re-embed jobs to target explicit model IDs and write into corresponding model tables.
- [ ] REFACTOR: improve batching/error handling/observability.

### Step 10: Bootstrap Existing Models and Data

- [ ] RED: integration tests for bootstrap creating tables for currently active models.
- [ ] GREEN: add bootstrap/backfill routine to seed new tables from existing rows.
- [ ] REFACTOR: add resumability/checkpointing for large datasets.

### Step 11: Cutover Controls

- [ ] RED: tests for feature flags or mode toggles (`legacy-only`, `dual`, `new-only`).
- [ ] GREEN: implement runtime cutover controls and defaults.
- [ ] REFACTOR: simplify compatibility branches and remove dead code paths where safe.

### Step 12: Legacy Deprecation (Explicit Sign-off Required)

- [ ] RED: migration tests for dropping legacy embedding columns/indexes after full cutover.
- [ ] GREEN: add cleanup migration(s), guarded by operator sign-off milestone.
- [ ] REFACTOR: remove legacy fallback code and associated flags.

### Step 13: Documentation and Runbooks

- [ ] RED: docs completeness checklist test (manual gating item).
- [ ] GREEN: publish user/admin docs for registration, activation, re-embedding, rollback, and troubleshooting.
- [ ] REFACTOR: align terminology across CLI, API, schema, and docs.

### Step 14: Final Validation Gate

- [ ] Run full unit + integration suites with both providers enabled in matrix.
- [ ] Validate performance/index health on representative data volume.
- [ ] Confirm no scope-auth regressions in recall/query flows.
- [ ] Mark migration as complete and archive remaining deferred items as follow-up tasks.
