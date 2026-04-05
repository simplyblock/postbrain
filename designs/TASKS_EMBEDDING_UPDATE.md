# TASKS — Embedding Architecture Update

## Goal

Move from fixed inline embedding columns to a model-scoped embedding architecture that supports:
- one embedding storage table per embedding model/dimension contract,
- multiple embedding providers/services in parallel,
- safe model registration and activation,
- re-embedding across providers without breaking recall of existing embeddings.

---

    ## Resolved Design Decisions

### Table Naming Convention
`embeddings_model_<uuid_no_dashes>` — deterministic, collision-free, safe SQL identifier.

### Central Embedding Metadata Table (`embedding_index`)

Tracks which objects have embeddings in which model tables, including backfill status:

```sql
CREATE TABLE embedding_index (
    object_type  TEXT        NOT NULL CHECK (object_type IN ('memory', 'entity', 'knowledge_artifact', 'skill')),
    object_id    UUID        NOT NULL,
    model_id     UUID        NOT NULL REFERENCES embedding_models(id),
    status       TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'ready', 'failed')),
    retry_count  INT         NOT NULL DEFAULT 0,
    last_error   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY  (object_type, object_id, model_id)
);
```

- `status = 'pending'`: embedding has not yet been written to the model table.
- `status = 'ready'`: embedding is present and queryable.
- `status = 'failed'`: exhausted 3 retry attempts; requires manual reset.
- The re-embed job processes `pending` rows, increments `retry_count` on failure, and sets `failed` after 3 attempts.
- When a new model is registered, the registration flow inserts `pending` rows for **all existing objects**.

### Per-Model Vector Table Schema

One table per registered model. `N = embedding_models.dimensions`.

```sql
CREATE TABLE embeddings_model_<uuid> (
    object_id  UUID      NOT NULL,
    embedding  vector(N) NOT NULL,
    PRIMARY KEY (object_id)
);
CREATE INDEX ON embeddings_model_<uuid>
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);
```

`object_type` is not repeated in the vector table — it is implicit via the model's `content_type` and is stored in `embedding_index`.

### Object Linkage Strategy
Central `embedding_index` table + per-model vector tables. Legacy `embedding_model_id` FK columns in `memories`, `entities`, `knowledge_artifacts`, and `skills` are retained during compatibility and removed in a later cleanup migration.

### Multi-Content-Type Per Object
A memory can have both a text embedding and a code embedding. In the new architecture it gets one row per model in `embedding_index` and one row per model in the corresponding per-model vector table. The `content_type` on `embedding_models` encodes the distinction.

### `embedding_models` Schema Extension
New columns added to `embedding_models`:
- `slug` — operator-assigned unique human-readable identifier (e.g. `"my-text-model-v2"`). Retained as the unique key.
- `provider_model` — the model name sent to the provider API (e.g. `"nomic-embed-text"`). Can differ from `slug`.
- `provider` — provider runtime (`"ollama"` or `"openai"`).
- `service_url` — per-model endpoint URL.
- `table_name` — name of the backing per-model vector table, set at registration time.
- `is_ready` — false until registration transaction completes successfully.

The existing `is_active` partial unique indexes (one active model per `content_type`) are **preserved unchanged**.

### EmbeddingService / Factory Integration
`EmbeddingService` stays as the call site for all write paths. It wraps an internal `EmbedderForModel(modelID)` factory. `EmbedText` and `EmbedCode` return:

```go
type EmbedResult struct {
    ModelID   uuid.UUID
    Embedding []float32
}
```

Write paths receive both the vector and the `model_id` in one call without knowing about the factory.

### Repository Scope Filtering
The embedding repository owns scope-filtered ANN join queries per object type. Callers pass a typed filter:

```go
type EmbeddingQuery struct {
    ModelID    uuid.UUID
    ObjectType string
    Embedding  []float32
    Limit      int
    Scope      *ScopeFilter
}

type ScopeFilter struct {
    ScopePath  string
    AgentType  string
    Visibility []string
}
```

The repository joins back to the source object table to apply scope filters. No raw predicate injection from callers.

### Repository Caching
No cache — the repository always queries the DB for model/table lookup. Add caching only if profiling shows it is necessary.

### Bootstrap Strategy
Bootstrap (Step 8) is a **data migration** — it copies existing legacy vectors from inline columns into the new per-model tables. No provider calls. Only applies to models with existing legacy data. New models with no legacy data are handled by the re-embed job.

### Fallback Retrieval Policy
Fallback only when the active-model embedding is missing (`status != 'ready'` in `embedding_index`). No cross-model score blending in a single ranking pass.

### Cutover Strategy
Fast/aggressive. Cutover progression is **deploy-gated**, not flag-gated:
1. Deploy dual-write (Step 7) — new tables populated going forward.
2. Run bootstrap (Step 8) — backfill existing data.
3. Deploy dual-read (Step 9) — new tables preferred, legacy fallback.
4. Deploy legacy deprecation (Step 11) — legacy columns dropped after sign-off.

No runtime toggle required.

### Legacy Column Deprecation Sign-off Criteria
All of the following must be true before Step 11 begins:
1. Step 13 final validation gate passes (full integration suite, both providers, scope-auth regression checks).
2. Zero `status = 'pending'` rows in `embedding_index` for all active models.
3. Zero `status = 'failed'` rows, or all failures reviewed and explicitly accepted.
4. Dual-read has been running in production for at least one full re-embed cycle with no fallback hits logged.

---

## Phase 1: Schema & Contracts

- [ ] Define and approve the storage contract:
  - One physical embedding table per registered embedding model.
  - Table schema uses fixed `vector(N)` where `N = embedding_models.dimensions`.
  - One model row maps to exactly one provider/service/model slug and one dimension contract.

- [ ] Extend `embedding_models` schema:
  - Add `provider`, `service_url`, `provider_model`, `table_name`, `is_ready`, `created_at` columns.
  - Add constraints/indexes for uniqueness and safe lifecycle transitions.
  - Preserve existing `is_active` partial unique indexes unchanged.

- [ ] Introduce `embedding_index` table per resolved schema above.

- [x] Keep `embedding_model_id` FK columns during compatibility phase; cleanup deferred to dedicated migration.

- [x] Define naming and validation rules for per-model tables:
  - deterministic table name from UUID (strip dashes),
  - SQL identifier safety guaranteed by UUID character set.

---

## Phase 2: Migrations & Bootstrap

- [x] Add non-breaking migration(s) to introduce new metadata columns and `embedding_index`.

- [ ] Add bootstrap logic to copy legacy inline vectors into new per-model tables and mark `embedding_index` rows as `ready`.

- [ ] Add verification SQL checks:
  - table exists for every `is_ready = true` model,
  - dimensions in table type match `embedding_models.dimensions`.

---

## Phase 3: Embedding Storage Abstraction

- [ ] Introduce embedding repository layer in `internal/db` using `EmbeddingQuery` / `ScopeFilter` API.

- [ ] Implement core operations:
  - upsert embedding for `(object_type, object_id, model_id)`,
  - fetch embedding by object + model,
  - ANN query per scope/object type/model table with `ScopeFilter`.

- [ ] Enforce strict dimension checks at repository boundary.

- [ ] Repository always queries DB for model/table lookup — no cache.

---

## Phase 4: Multi-Provider Embedder Factory

- [ ] Replace single global backend with model-driven provider resolution:
  - `EmbedderForModel(modelID)` builds provider client from model config.

- [ ] Support:
  - `provider=ollama` with per-model `service_url`,
  - `provider=openai` with per-model `service_url` + optional auth.

- [ ] `EmbeddingService` wraps factory; `EmbedText`/`EmbedCode` return `EmbedResult{ModelID, Embedding}`.

---

## Phase 5: Model Registration & CLI

- [ ] Implement registration as a single transaction:
  - insert model metadata,
  - CREATE TABLE + HNSW index for that model,
  - set `table_name` and `is_ready = true`,
  - insert `pending` rows in `embedding_index` for all existing objects.
  - Idempotency via `ON CONFLICT (slug)`. Rollback on any failure — no cleanup logic needed.

- [ ] Add CLI commands:
  - `embedding-model register` (provider, service_url, provider_model, slug, dimensions, content_type, optional activate),
  - `embedding-model activate`,
  - `embedding-model list`.

---

## Phase 6: Query/Recall Refactor

- [ ] Update memory/knowledge/skills/entity recall to query via repository with `ScopeFilter`.

- [ ] Default retrieval: query active model for the content type.

- [ ] Fallback when active-model embedding is missing: fallback to prior active model and/or trigger async backfill.

- [ ] Keep score handling explicit; no cross-model score blending.

---

## Phase 7: Re-Embedding Pipeline

- [ ] Refactor re-embed job to target a specific model ID.

- [ ] Re-embed flow:
  - process `embedding_index` rows WHERE `status = 'pending'` for target model,
  - load raw content,
  - embed via target model provider config,
  - write embedding to target model table,
  - set `status = 'ready'`,
  - on failure: increment `retry_count`; set `status = 'failed'` after 3 attempts.

- [ ] Manual reset required to retry `failed` rows.

---

## Phase 8: Compatibility & Cutover

- [ ] Phase A (dual-write): write to new model tables while legacy columns are still present.
- [ ] Phase B (bootstrap): copy legacy vectors into new model tables.
- [ ] Phase C (dual-read): read from new tables first, fallback legacy vector data if `embedding_index` row missing/not ready.
- [ ] Phase D (cleanup migration): drop legacy embedding vector columns/indexes after explicit sign-off per criteria above.

---

## Phase 9: Testing Strategy (TDD-first)

- [ ] Unit tests:
  - table-name generation and sanitization,
  - dimension contract enforcement,
  - provider factory resolution,
  - registration lifecycle validation,
  - `EmbedResult` returned correctly from `EmbeddingService`.

- [ ] Integration tests:
  - register model → `embedding_index` pending rows inserted,
  - registration transaction rollback on failure,
  - write/read/ANN queries through model tables with scope filters,
  - multi-provider coexistence,
  - re-embed retry and failure behavior,
  - bootstrap vector copy correctness.

- [ ] Regression tests:
  - scope-locked recall correctness,
  - MCP/REST behavior unchanged from caller perspective.

---

## Phase 10: Ops & Documentation

- [ ] Update user docs:
  - model registration workflow,
  - provider/service configuration per model,
  - activation and rollback procedures,
  - re-embedding runbook,
  - manual reset procedure for `failed` embedding_index rows.

- [ ] Add operational checks:
  - index/table health verification,
  - `embedding_index` status dashboards/metrics,
  - migration rollback notes.

---

## Ordered Execution Plan (TDD-Gated)

Every step follows strict Red → Green → Refactor before moving to the next step.

### TDD Rules (Mandatory for Every Step)

- [ ] Write failing test(s) first (`RED`) for the exact behavior in the step.
- [ ] Confirm failure is real (test fails for expected reason).
- [ ] Implement the minimum code to pass (`GREEN`).
- [ ] Refactor while keeping tests green (`REFACTOR`).
- [ ] Run full gate before moving on:
  1. `make generate` (sqlc regeneration)
  2. `go test -tags integration ./...`
  3. `gofmt -w .`
  4. `make lint`
- [ ] Update this tasks file status before commit.

---

### Step 1: Schema Preparation

- [x] RED: migration tests for new `embedding_models` columns (`provider`, `service_url`, `provider_model`, `table_name`, `is_ready`) and `embedding_index` table, with legacy FK columns retained during compatibility.
- [x] GREEN: add migration(s) implementing the above. Preserve existing `is_active` partial unique indexes.
- [ ] REFACTOR: tighten constraints/indexes and migration comments.

### Step 2: Dynamic Table Provisioning Primitives

- [x] RED: tests for safe table-name generation (`embeddings_model_<uuid_no_dashes>`) and per-model table + HNSW index creation.
- [x] GREEN: implement helper(s) to generate table name from UUID and execute CREATE TABLE + index DDL.
- [ ] REFACTOR: isolate SQL generation for reuse in registration flow.

### Step 3: Model Registration Backend Flow

- [x] RED: tests for registration transaction behavior (insert model, CREATE TABLE + index, set `table_name`/`is_ready`, insert `pending` rows in `embedding_index` for all existing objects). Test rollback on failure. Test `ON CONFLICT (slug)` idempotency.
- [x] GREEN: implement registration as a single transaction; no compensating cleanup needed.
- [x] REFACTOR: split validation/provisioning/persistence responsibilities.

### Step 4: CLI Command for Model Registration

- [x] RED: command tests for argument validation and success/failure output.
- [x] GREEN: add `embedding-model register`, `embedding-model activate`, `embedding-model list` commands.
- [ ] REFACTOR: share parsing/validation across embedding-model commands.

### Step 5: Multi-Provider Embedder Factory

- [x] RED: tests for `EmbedderForModel(modelID)` provider resolution, `service_url` routing, and `EmbedResult` returned by `EmbeddingService.EmbedText`/`EmbedCode`.
- [x] GREEN: implement factory for `ollama` and `openai`; update `EmbeddingService` to wrap factory and return `EmbedResult`.
- [ ] REFACTOR: unify client construction and error messaging.

### Step 6: Embedding Repository Layer

- [x] RED: tests for per-model-table upsert/get/ANN routing, dimension enforcement, and `ScopeFilter` application per object type.
- [x] GREEN: add repository API in `internal/db` with `EmbeddingQuery` / `ScopeFilter`; repository joins back to source object tables for scope filtering; always queries DB for model/table lookup (no cache).
- [x] REFACTOR: centralize model/table lookup and retry-safe DB access.
  - Progress: added shared model metadata lookup helpers and converted repository writes to retry-safe transactional upserts (`serialization_failure`/`deadlock_detected` retry semantics), with focused unit + integration coverage.

### Step 7: Write Path Integration (Dual-Write Start)

- [x] RED: tests ensuring memory/knowledge/skills/entity writes store embeddings in new model table and insert/update `embedding_index` row to `ready`.
- [x] GREEN: integrate repository into write paths using `EmbedResult.ModelID`.
- [x] REFACTOR: reduce duplication across memory/knowledge/synthesis write paths.
  - Progress: introduced shared dual-write helper (`db.UpsertEmbeddingIfPresent`) and refactored memory/knowledge/skills stores to use it; helper behavior covered with unit tests.

### Step 8: Bootstrap / Backfill

 - [x] RED: integration tests for bootstrap copying legacy inline vectors into new per-model tables and marking `embedding_index` rows as `ready`.
 - [x] GREEN: add bootstrap routine that reads legacy vector columns and writes to per-model tables; no provider calls.
- [x] REFACTOR: add resumability (skip already-`ready` rows) and progress logging.
  - Progress: bootstrap queries now skip rows already marked `ready` for the target model and emit per-stage/final progress logs; second-run skip behavior covered by integration test.

### Step 9: Read/Recall Path Integration (Dual-Read)

- [ ] RED: tests ensuring recall queries new model tables via repository and falls back per approved policy when `embedding_index` row is missing or not `ready`.
- [ ] GREEN: route ANN retrieval through new repository with `ScopeFilter`.
- [ ] REFACTOR: normalize scoring interfaces; keep scope auth semantics unchanged.
  - Progress: memory/knowledge/skills vector recall now attempts active-model repository ANN first and falls back to legacy vector queries when model tables are unavailable or empty; integration coverage added for model-table-only recall paths in all three layers.

### Step 10: Re-Embed Pipeline Refactor

- [x] RED: tests for re-embed job processing `pending` rows for a target model ID, retry logic (up to 3), and `failed` status after exhaustion.
- [x] GREEN: refactor re-embed job to query `embedding_index WHERE status = 'pending' AND model_id = <target>`, call provider, write vector, update status.
- [ ] REFACTOR: improve batching/error handling/observability.
  - Progress: both `RunText` and `RunCode` now consume `embedding_index` pending rows, update legacy+model-table embeddings, mark `ready` on success, and increment retry/mark `failed` after max retries.

### Step 11: Legacy Deprecation (Explicit Sign-off Required)

All four sign-off criteria must be met before this step begins (see Resolved Design Decisions above).

- [ ] RED: migration tests for dropping legacy embedding vector columns/indexes.
- [ ] GREEN: add cleanup migration(s).
- [ ] REFACTOR: remove any remaining legacy fallback code paths.

### Step 12: Documentation and Runbooks

- [ ] RED: docs completeness checklist (manual gating item).
- [ ] GREEN: publish user/admin docs for registration, activation, re-embedding, rollback, and manual `failed`-row reset procedure.
- [ ] REFACTOR: align terminology across CLI, API, schema, and docs.

### Step 13: Final Validation Gate

- [ ] Run full unit + integration suites with both providers enabled in matrix.
- [ ] Validate performance/index health on representative data volume.
- [ ] Confirm no scope-auth regressions in recall/query flows.
- [ ] Confirm zero `pending` and `failed` rows in `embedding_index`.
- [ ] Mark migration as complete and archive remaining deferred items as follow-up tasks.
