# Test Coverage Tasks

Incremental plan for improving code coverage across the Go codebase.
Each item is self-contained and can be picked up independently.

Coverage baseline (integration run, 2026-04-01):

| Package                  | Coverage |
|--------------------------|----------|
| `internal/ui`            | 8.6 %    |
| `internal/sharing`       | 18.2 %   |
| `internal/jobs`          | 19.5 %   |
| `internal/api/rest`      | 21.2 %   |
| `internal/api/mcp`       | 22.8 %   |
| `internal/skills`        | 38.0 %   |
| `internal/knowledge`     | 45.2 %   |
| `internal/skills`        | 47.2 %   |
| `internal/retrieval`     | 56.8 %   |
| `internal/codegraph`     | 53.8 %   |
| `internal/auth`          | 59.1 %   |
| `internal/graph`         | 65.2 %   |
| `internal/memory`        | 65.2 %   |
| `internal/config`        | 68.2 %   |
| `internal/embedding`     | 77.1 %   |
| `internal/principals`    | 0.0 %    |
| `internal/db`            | 3.1 %    |

Legend: `[ ]` = todo · `[x]` = done

---

## Priority 1 — Pure logic, no DB required

### auth — TokenStore and middleware unit tests (`internal/auth/`)

Existing `tokens_test.go` only tests `GenerateToken`/`HashToken`.
`middleware_test.go` covers `bearerTokenMiddlewareWithStore` but leaves several
branches untested.

- [x] `TokenStore.Lookup` — nil token returned by `db.LookupToken` returns `nil, nil`
- [x] `TokenStore.Lookup` — revoked token (`RevokedAt` set) returns `nil, nil`
- [x] `TokenStore.Lookup` — expired token (`ExpiresAt` in past) returns `nil, nil`
- [x] `TokenStore.UpdateLastUsed` — nil pool is a no-op (no panic)
- [x] `bearerTokenMiddlewareWithStore` — `Bearer ` prefix but empty token string returns 401
- [x] `EnforceScopeAccess` — nil `ScopeIds` always returns nil (unrestricted token)
- [x] `EnforceScopeAccess` — non-nil list not containing requested scope returns error
- [x] `EnforceScopeAccess` — non-nil list containing requested scope returns nil

File: `internal/auth/tokens_test.go` (extend), `middleware_test.go` (extend), `tokens_integration_test.go` (new)

---

### sharing — grant validation unit tests (`internal/sharing/grants.go`)

`grants_test.go` covers the `ErrInvalidGrant` validation path.
The `Revoke`/`List`/`IsMemoryAccessible`/`IsArtifactAccessible` functions are
fully untested (all need a real DB).

- [x] `Create` — only MemoryID set: validation passes (no `ErrInvalidGrant`), nil-pool
  panic is expected (shows DB path reached)
- [x] `Create` — only ArtifactID set: validation passes (same pattern)
- [x] `Revoke` / `List` / `IsMemoryAccessible` / `IsArtifactAccessible` — move to
  `sharing_integration_test.go` (these are thin wrappers over raw SQL; one
  round-trip test each is sufficient)

File: `internal/sharing/grants_test.go` (extend), create `internal/sharing/grants_integration_test.go`

---

### knowledge — visibility deduplication unit tests (`internal/knowledge/visibility.go`)

`visibility_test.go` only tests `deduplicateScopeIDs`. `ResolveVisibleScopeIDs`
and `getPersonalScope` are 0 % covered.

- [x] `deduplicateScopeIDs` — empty input returns empty slice (not nil)
- [x] `deduplicateScopeIDs` — all-duplicate input returns single element
- [x] `deduplicateScopeIDs` — order preserved across duplicates
- [x] `ResolveVisibleScopeIDs` — root scope (no parent, no personal scope) returns exactly that scope
- [x] `ResolveVisibleScopeIDs` — child scope returns both child and parent IDs
- [x] `ResolveVisibleScopeIDs` — no personal scope: getPersonalScope nil-return path covered
  (Note: 'personal' is not a valid scope kind in the schema; personal scope path is unreachable)

File: `internal/knowledge/visibility_test.go` (extend), `internal/knowledge/visibility_integration_test.go` (new)

---

### knowledge — lifecycle state machine unit tests (`internal/knowledge/lifecycle.go`)

`lifecycle_test.go` has good `Endorse`/`autoPublish` coverage.
`RetractToDraft`, `Republish`, and `Delete` are at 0 %.

- [x] `RetractToDraft` — artifact not found returns `ErrInvalidTransition`
- [x] `RetractToDraft` — artifact not `in_review` returns `ErrInvalidTransition`
- [x] `RetractToDraft` — author can retract; transitions to `"draft"`
- [x] `RetractToDraft` — non-author non-admin returns `ErrForbidden`
- [x] `RetractToDraft` — admin (non-author) can retract; transitions to `"draft"`
- [x] `Republish` — artifact not `deprecated` returns `ErrInvalidTransition`
- [x] `Republish` — non-admin returns `ErrForbidden`
- [x] `Republish` — admin transitions to `"published"`
- [x] `Delete` — artifact not found returns `ErrInvalidTransition`
- [x] `Delete` — non-admin returns `ErrForbidden`
- [x] `Delete` — admin cascades all pre-delete steps (verify `nullPreviousVersionRefs`,
  `nullPromotionRequestArtifactRef`, `resetPromotedMemoryStatus` all called)

File: `internal/knowledge/lifecycle_test.go` (extend)

---

### skills — lifecycle state machine unit tests (`internal/skills/lifecycle.go`)

`lifecycle_test.go` covers `SubmitForReview` and `Endorse`. `RetractToDraft`,
`Deprecate`, `Republish`, and `EmergencyRollback` are at 0 %.

- [x] `RetractToDraft` — nil skill returns `ErrInvalidTransition`
- [x] `RetractToDraft` — wrong status returns `ErrInvalidTransition`
- [x] `RetractToDraft` — `in_review` skill transitions to `"draft"`
- [x] `Deprecate` — nil skill returns `ErrInvalidTransition`
- [x] `Deprecate` — wrong status returns `ErrInvalidTransition`
- [x] `Deprecate` — non-admin returns `ErrForbidden`
- [x] `Deprecate` — published skill transitions to `"deprecated"` (admin)
- [x] `Republish` — nil skill returns `ErrInvalidTransition`
- [x] `Republish` — wrong status returns `ErrInvalidTransition`
- [x] `Republish` — non-admin returns `ErrForbidden`
- [x] `Republish` — deprecated skill transitions to `"published"` (admin)
- [x] `EmergencyRollback` — nil skill returns `ErrInvalidTransition`
- [x] `EmergencyRollback` — already-draft returns `ErrInvalidTransition`
- [x] `EmergencyRollback` — non-admin returns `ErrForbidden`
- [x] `EmergencyRollback` — admin transitions any non-draft skill to `"draft"` (table-driven)

File: `internal/skills/lifecycle_test.go` (extend)

---

### skills — recall scoring helpers (`internal/skills/recall.go`)

`recall_test.go` covers `computeSkillScore`. `importanceFromInvocations` is 0 %.

- [x] `importanceFromInvocations(0)` → 0.0
- [x] `importanceFromInvocations(50)` → 0.5
- [x] `importanceFromInvocations(100)` → 1.0 (exact boundary)
- [x] `importanceFromInvocations(200)` → 1.0 (capped)

File: `internal/skills/recall_test.go` (extend)

---

### retrieval — CosineSimilarity (`internal/retrieval/merge.go`)

`CosineSimilarity` is 0 % covered.

- [x] Two identical unit vectors → 1.0
- [x] Orthogonal vectors → 0.0
- [x] Zero vector (denominator guard) → 0.0 (no panic/NaN)
- [x] Negative dot product → result clamped at 0 or negative (verify behavior)

File: `internal/retrieval/merge_test.go` (extend existing)

---

## Priority 2 — Requires test DB (`testcontainers`)

Mark files `//go:build integration`.

### principals — CRUD + membership integration tests

`store_test.go` uses `TEST_DATABASE_URL`; migrate it to `testcontainers` for
consistency. `membership_test.go` tests only the cycle-detection logic without a DB.

- [x] `Store.Create` / `GetByID` / `GetBySlug` / `Update` / `List` / `Delete` —
  full round-trip using `testhelper.NewTestPool`; replace env-var-gated
  `testPool` helper with `testhelper.NewTestPool` (use `//go:build integration`)
- [x] `MembershipStore.AddMembership` — valid role inserts membership
- [x] `MembershipStore.AddMembership` — cycle detection with real ancestor query
- [x] `MembershipStore.RemoveMembership` — removes the record
- [x] `MembershipStore.EffectiveScopeIDs` — returns scopes for principal and its parents
- [x] `MembershipStore.IsScopeAdmin` — scope owner is admin; explicit admin role is admin;
  member role is not admin

File: `internal/principals/store_integration_test.go` (new), `membership_integration_test.go` (new)

---

### sharing — grant round-trip integration tests (`internal/sharing/grants.go`)

- [x] `Create` with memory grant — inserted record scannable, fields round-trip
- [x] `Create` with artifact grant — same
- [x] `Revoke` — deletes the record; subsequent `List` does not return it
- [x] `List` — returns grants for the grantee scope, pagination works
- [x] `IsMemoryAccessible` — true when grant exists and not expired; false when expired
- [x] `IsArtifactAccessible` — true when grant exists; false when no grant

File: `internal/sharing/grants_integration_test.go` (new)

---

### knowledge — collections integration tests (`internal/knowledge/collections.go`)

`CollectionStore` methods `GetByID`, `GetBySlug`, `List`, `AddItem`, `RemoveItem`,
`ListItems` are all 0 %.

- [x] `Create` → `GetByID` round-trip
- [x] `GetBySlug` — returns same record as `GetByID`
- [x] `List` — returns created collection(s) for the scope
- [x] `AddItem` / `ListItems` — artifact appears in list after `AddItem`
- [x] `RemoveItem` — artifact absent from list after `RemoveItem`

File: `internal/knowledge/collections_integration_test.go` (new)

---

### knowledge — visibility integration tests (`internal/knowledge/visibility.go`)

- [x] `ResolveVisibleScopeIDs` — scope with parent: both IDs in result
- [x] `ResolveVisibleScopeIDs` — principal has personal scope: personal scope ID appended
  (NOTE: `kind='personal'` is not a valid schema value; personal scope branch is dead code;
  covered by the NoPersonalScope test which exercises the nil-return path instead)
- [x] `ResolveVisibleScopeIDs` — no personal scope: result is just ancestor chain

File: `internal/knowledge/visibility_integration_test.go` (new)

---

### knowledge — Lifecycle integration tests for missing transitions

`lifecycle_integration_test.go` covers `Endorse`/`AutoPublish` end-to-end.
The `RetractToDraft`, `Republish`, `Delete` transitions need a real artifact.

- [x] `SubmitForReview` → `RetractToDraft` → verify status `"draft"`
- [x] `Republish` — create artifact, publish, deprecate, then republish; status `"published"`
- [x] `Delete` — artifact + cascade records removed; subsequent `GetByID` returns nil

File: `internal/knowledge/lifecycle_integration_test.go` (extend)

---

### jobs — reembed integration test (`internal/jobs/reembed.go`)

`RunText` and `RunCode` are 0 % covered. They batch-fetch records by comparing
`embedding_model_id` to the active model.

- [x] `RunText` with no active text model: returns nil without touching rows
- [x] `RunText` with active model and one mismatched memory: row gets re-embedded
- [x] `RunCode` with active model and one mismatched code memory: row updated

File: `internal/jobs/reembed_integration_test.go` (new, `//go:build integration`)

---

### jobs — staleness / contradiction integration test (`internal/jobs/staleness.go`)

The `ContradictionJob.Run` / `fetchArtifactBatch` / `processArtifact` /
`fetchRecentMemories` / `filterByTopicSimilarity` are all 0 %.

- [x] `Run` on empty DB — returns nil, no panics
- [x] `Run` with one published artifact and one recent memory that contradicts:
  verify `FlagArtifactStaleness` was called (inject fake classifier that returns
  `"CONTRADICTS"`)
- [x] `filterByTopicSimilarity` — memory whose cosine distance > threshold is filtered out

File: `internal/jobs/staleness_integration_test.go` (new)

---

### skills — Recall integration test (`internal/skills/recall.go`)

`Store.Recall` is 0 % covered end-to-end.

- [x] Empty query returns empty slice (no panic)
- [x] Single published skill appears in results when query matches title
- [x] `Installed` filter: `true` returns only installed skills; `false` returns only uninstalled
- [x] `Limit` is respected: more than `Limit` candidates → result capped

File: `internal/skills/skills_integration_test.go` (extend existing)

---

### skills — store Update/GetBySlug/GetByID integration tests (`internal/skills/store.go`)

- [x] `Update` — changes title and description; returns updated record
- [x] `GetBySlug` — returns the correct skill; unknown slug returns nil
- [x] `GetByID` — returns the correct skill; unknown ID returns nil

File: `internal/skills/skills_integration_test.go` (extend existing)

---

## Priority 3 — REST handler coverage gaps (`internal/api/rest/`)

All handlers below are at 0 %; they follow the same httptest pattern used in
existing tests. Pass `nil` pool where DB errors are acceptable.

### orgs handlers (`internal/api/rest/orgs.go`)

- [x] `POST /v1/orgs` — missing `slug` returns 400
- [x] `GET /v1/orgs` — returns 401 without auth (full router path)
- [x] `GET /v1/orgs/:id` — invalid UUID returns 400
- [x] `PUT /v1/orgs/:id` — missing body / invalid UUID returns 400
- [x] `DELETE /v1/orgs/:id` — invalid UUID returns 400

File: `internal/api/rest/orgs_test.go` (new)

---

### sessions handlers (`internal/api/rest/sessions.go`)

- [x] `POST /v1/sessions` — missing `scope` returns 400
- [x] `POST /v1/sessions` — invalid scope format returns 400
- [x] `POST /v1/sessions` — invalid JSON returns 400
- [x] `POST /v1/sessions` — no auth returns 401
- [x] `PATCH /v1/sessions/:id` — invalid UUID returns 400
- [x] `PATCH /v1/sessions/:id` — invalid JSON returns 400
- [x] `PATCH /v1/sessions/:id` — invalid ended_at returns 400
- [x] `PATCH /v1/sessions/:id` — no auth returns 401

File: `internal/api/rest/sessions_test.go` (new)

---

### sharing handlers (`internal/api/rest/sharing.go`)

- [x] `POST /v1/sharing/grants` — missing grantee_scope_id returns 400
- [x] `POST /v1/sharing/grants` — invalid grantee_scope_id/memory_id/artifact_id/expires_at returns 400
- [x] `POST /v1/sharing/grants` — invalid JSON returns 400
- [x] `POST /v1/sharing/grants` — no auth returns 401
- [x] `GET /v1/sharing/grants` — invalid grantee_scope_id query param returns 400
- [x] `GET /v1/sharing/grants` — no auth returns 401
- [x] `DELETE /v1/sharing/grants/:id` — invalid UUID returns 400
- [x] `DELETE /v1/sharing/grants/:id` — no auth returns 401

File: `internal/api/rest/sharing_test.go` (new)

---

### collections handlers (`internal/api/rest/collections.go`)

- [ ] `POST /v1/collections` — missing `slug` returns 400
- [ ] `GET /v1/collections/:id` — invalid UUID returns 400
- [ ] `POST /v1/collections/:id/items` — missing `artifact_id` returns 400
- [ ] `DELETE /v1/collections/:id/items/:artifact_id` — invalid artifact UUID returns 400

File: `internal/api/rest/collections_test.go` (new)

---

### synthesis handlers (`internal/api/rest/synthesis.go`)

- [ ] `POST /v1/synthesis` — missing `scope` returns 400
- [ ] `GET /v1/synthesis/:id` — invalid UUID returns 400

File: `internal/api/rest/synthesis_test.go` (new)

---

### skills handlers (`internal/api/rest/skills.go`)

- [ ] `POST /v1/skills` — missing `title` returns 400
- [ ] `GET /v1/skills/recall` — missing `q` returns 400
- [ ] `GET /v1/skills/:id` — invalid UUID returns 400
- [ ] `POST /v1/skills/:id/review` — invalid UUID returns 400
- [ ] `POST /v1/skills/:id/endorse` — invalid UUID returns 400

File: `internal/api/rest/skills_test.go` (new)

---

### upload handler (`internal/api/rest/upload.go`)

- [ ] `POST /v1/upload` — no multipart form returns 400
- [ ] `POST /v1/upload` — valid form but nil pool: handler handles gracefully (no panic)

File: `internal/api/rest/upload_test.go` (new)

---

### graph handler remaining branches (`internal/api/rest/graph.go`)

`/v1/graph/callers`, `/v1/graph/callees`, `/v1/graph/dependencies`,
`/v1/graph/dependents` are 0 %.

- [ ] `GET /v1/graph/callers` — missing `symbol` returns 400
- [ ] `GET /v1/graph/callees` — missing `scope` returns 400
- [ ] `GET /v1/graph/dependencies` — missing `symbol` returns 400
- [ ] `GET /v1/graph/dependents` — missing `scope` returns 400

File: `internal/api/rest/graph_handlers_test.go` (new, extend existing `graph_helpers_test.go` or new file)

---

## Priority 4 — MCP handler coverage gaps (`internal/api/mcp/`)

### mcp — endorse, promote, knowledge-detail, list-scopes, collect, session, skill handlers

All below are at 0 %. The existing `handlers_unit_test.go` and `server_test.go`
pattern (nil pool, direct tool handler call with injected context) applies.

- [ ] `handleEndorse` — missing `artifact_id` param returns tool error
- [ ] `handleEndorse` — invalid UUID returns tool error
- [ ] `handlePromote` — missing `memory_id` returns tool error
- [ ] `handleKnowledgeDetail` — missing `artifact_id` returns tool error
- [ ] `handleListScopes` — succeeds with nil pool (returns tool error gracefully, not panic)
- [ ] `handleCollect` — missing `scope` param returns tool error
- [ ] `handleSynthesise` — missing `scope` param returns tool error
- [ ] `handleSkillSearch` — missing `query` param returns tool error
- [ ] `handleSkillInstall` — missing `slug` param returns tool error
- [ ] `handleSkillInvoke` — missing `slug` param returns tool error

File: `internal/api/mcp/handlers_unit_test.go` (extend)

---

## Priority 5 — UI handler coverage gaps (`internal/ui/`)

`internal/ui` is at 8.6 %. The handler has many routes that are untested.
Use `httptest` with a nil-pool `Handler`.

### auth flow (`internal/ui/auth.go`, `tokens.go`)

- [ ] `GET /ui/login` — renders login form (200)
- [ ] `POST /ui/login` — missing token field returns form error (not 500)
- [ ] `GET /ui/logout` — clears session cookie and redirects
- [ ] `GET /ui/tokens` — unauthenticated redirects to login
- [ ] `POST /ui/tokens` — missing name field returns form error

File: `internal/ui/handler_auth_test.go` (new)

---

### knowledge CRUD UI (`internal/ui/handler.go`)

- [ ] `GET /ui/knowledge/new` — renders form (200)
- [ ] `POST /ui/knowledge` — missing title returns form error
- [ ] `GET /ui/knowledge/:id` — invalid UUID returns 400 or redirect
- [ ] `POST /ui/knowledge/:id/submit` — invalid UUID returns 400
- [ ] `POST /ui/knowledge/:id/retract` — invalid UUID returns 400
- [ ] `POST /ui/knowledge/:id/endorse` — invalid UUID returns 400
- [ ] `POST /ui/knowledge/:id/deprecate` — invalid UUID returns 400
- [ ] `POST /ui/knowledge/:id/delete` — invalid UUID returns 400

File: `internal/ui/handler_knowledge_test.go` (extend existing)

---

### memory UI (`internal/ui/handler.go`)

- [ ] `GET /ui/memories` — renders without error (nil pool)
- [ ] `GET /ui/memories/:id` — invalid UUID returns 400
- [ ] `POST /ui/memories/:id/forget` — invalid UUID returns 400

File: `internal/ui/handler_memories_test.go` (new)

---

### collections UI (`internal/ui/handler.go`)

- [ ] `GET /ui/collections` — renders list (nil pool, empty result)
- [ ] `GET /ui/collections/new` — renders form (200)
- [ ] `POST /ui/collections` — missing name returns form error

File: `internal/ui/handler_collections_test.go` (new)

---

## General guidelines

- **Use `embedding.FakeEmbedder`** for all unit tests that need an embedder —
  `embedding.NewFakeEmbedder(dims)` is deterministic, normalized, dependency-free.
  Wrap for `EmbeddingService`: `embedding.NewServiceFromEmbedders(embedding.NewFakeEmbedder(dims), nil)`.

- **Use `noopXxx` base structs for interface fakes** — embed a `noopXxx` struct
  that satisfies the full interface, override only the methods under test.
  See `noopLifecycleDB` in `lifecycle_test.go` as the reference implementation.

- **Integration test build tag** — any test requiring a live Postgres must be
  tagged `//go:build integration`.

- **Test file naming** — unit tests in `*_test.go`; integration tests in
  `*_integration_test.go`.

- **Table-driven tests** — use `[]struct{ name, input, want }` for functions
  with multiple input variants.

- **No mocks for business logic** — prefer real types with nil/stub dependencies;
  reserve mocks for I/O boundaries.

- **No test helpers without assertions** — every helper calls `t.Helper()` and
  `t.Fatal`/`t.Error` directly.
