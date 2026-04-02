# Scope Security Check — Remediation Tasks

Date: 2026-04-02
Status: In Progress
Owner: Engineering

## Goal

Ensure every scope-locked query and endpoint enforces scope authorization correctly, and that authorization supports multi-hop principal membership chains (`user -> team -> department -> company`).

---

## Finding 1: `EnforceScopeAccess` exists but is not applied in handlers

Risk: Critical  
Impact: Token `scope_ids` restrictions can be bypassed by providing arbitrary scope strings.

### Tasks

- [x] Add shared authorization helper primitive with unit tests first:
  - `internal/api/scopeauth.AuthorizeRequestedScope(token, requestedScopeID, effectiveScopeIDs)`
  - Enforces:
    - token `scope_ids` restrictions (`auth.EnforceScopeAccess`)
    - principal effective-scope inclusion
  - Added sentinel errors:
    - `ErrTokenScopeDenied`
    - `ErrPrincipalScopeDenied`
- [x] Add MCP equivalent helper (or shared package helper) and call it from all tools that accept `scope`.
- [x] Add unit tests for helper behavior:
  - nil token scope list allows all
  - explicit token scope list denies out-of-list scope
  - principal effective scope denies out-of-chain scope
  - combined checks require both to pass
- [x] Add context-aware adapter helper:
  - `authorizeRequestedScope(ctx, requestedScopeID)` (REST/MCP integration)
  - Pulls token from `auth.ContextKeyToken`
  - Pulls principal from `auth.ContextKeyPrincipalID`
  - Resolves effective scopes via `MembershipStore.EffectiveScopeIDs`
- [x] Wire scope authorization in all currently identified scope-string handlers (REST + MCP):
  - REST: `createMemory`, `recallMemories`, `promoteMemory`, `handleSummarizeMemories`, `createArtifact`, `searchArtifacts`, `createSkill`, `searchSkills`, `createCollection`, `listCollections`, `getCollection` (slug branch), `getContext`, `uploadKnowledge`, `createSession`, `listPromotions`
  - MCP: `remember`, `publish`, `recall`, `context`, `skill_search`, `promote`, `collect` (`add_to_collection` slug path, `create_collection`, `list_collections`), `session_begin`, `summarize`, `synthesize_topic`, `skill_install` (slug+scope path), `skill_invoke`
  - Guarded by inventory tests:
    - `internal/api/rest/scopeauth_inventory_test.go`
    - `internal/api/mcp/scopeauth_inventory_test.go`

### Acceptance Criteria

- Every scope-resolving endpoint/tool invokes helper before DB writes/reads.
- Tests prove deny-by-default for out-of-scope requests.

---

## Finding 2: Scope-bound write endpoints validate only scope existence

Risk: Critical  
Impact: Caller can write to scopes they should not access.

### Affected endpoints (observed)

- REST:
  - `POST /v1/memories`
  - `POST /v1/knowledge`
  - `POST /v1/skills`
  - `POST /v1/collections`
  - `POST /v1/knowledge/upload`
  - `POST /v1/sessions`
- MCP:
  - `remember`
  - `publish`
  - `context`
  - `recall`
  - `skill_search`
  - `promote`
  - `collect` (`create_collection`, `list_collections`, `add_to_collection` with `collection_slug`)
  - `session_begin`
  - `summarize`
  - `synthesize_topic`
  - `skill_install` (slug+scope path)
  - `skill_invoke`

### Tasks

- [x] For each endpoint/tool above, after scope resolution:
  - call `authorizeRequestedScope(...)`
  - return `403` (REST) or tool error (MCP) on unauthorized scope
- [x] Add table-driven REST tests:
  - authorized scope -> success
  - unauthorized scope -> forbidden
  - malformed scope -> bad request
- [x] Add MCP tests for unauthorized scope behavior.
  - Added integration tests:
    - `internal/api/rest/scope_authz_integration_test.go`
    - `internal/api/mcp/scope_authz_integration_test.go`
  - Expanded MCP matrix to all currently identified scope-taking tools and collect actions.

### Acceptance Criteria

- No write path proceeds with only `scope exists`; all require authorization pass.
- Integration coverage asserts authorized/unauthorized/malformed scope behavior for REST write endpoints and MCP scope-taking tools.

---

## Finding 3: ID-based read/update endpoints lack explicit scope checks

Risk: High  
Impact: Direct UUID access may bypass scope boundaries.

### Affected endpoints (observed)

- `GET /v1/memories/{id}`
- `PATCH /v1/memories/{id}`
- `DELETE /v1/memories/{id}`
- `GET /v1/knowledge/{id}`
- `PATCH /v1/knowledge/{id}`
- `GET /v1/skills/{id}`
- `PATCH /v1/skills/{id}`
- `GET /v1/collections/{id-or-slug}`
- collection item mutation endpoints

### Tasks

- [x] Introduce ownership-scope authorization checks after fetching object by ID:
  - memory: check `memory.scope_id`
  - artifact: check `artifact.owner_scope_id`
  - skill: check `skill.scope_id`
  - collection: check `collection.scope_id`
- [x] Add helper:
  - `authorizeObjectScope(ctx, objectScopeID) error`
- [x] Add regression tests per endpoint:
  - out-of-scope ID returns `403`, not `404/200`
  - in-scope ID remains successful
  - Added REST integration matrix in `internal/api/rest/scope_authz_integration_test.go` for:
    - `GET/PATCH/DELETE /v1/memories/{id}`
    - `GET/PATCH /v1/knowledge/{id}`
    - `GET/PATCH /v1/skills/{id}`
    - `GET /v1/collections/{id}`
    - `POST/DELETE /v1/collections/{id}/items...`

### Acceptance Criteria

- UUID-based operations cannot cross scope boundaries for currently identified ID-based handlers.

---

## Finding 4: Multi-hop principal chain logic is not used for API authorization

Risk: High  
Impact: Membership hierarchy exists but does not protect runtime endpoint access.

### Tasks

- [x] Wire `MembershipStore.EffectiveScopeIDs` into authorization helper.
- [x] Cache effective scopes in request context for reuse within a request.
  - Added `scopeauth.WithEffectiveScopeIDs(...)` / `EffectiveScopeIDsFromContext(...)`.
  - Added REST middleware preloading effective scopes into request context.
- [x] Add integration tests for chain authz:
  - `user -> team -> company`
  - allow self + ancestors, deny descendants + unrelated branches
- [x] Add tests with role variants (`member`, `owner`, `admin`) to confirm visibility inheritance behavior.
  - Added REST integration matrix in `internal/api/rest/scope_authz_integration_test.go` using `POST /v1/sessions`.
  - Added scopeauth unit test ensuring cached effective scopes bypass resolver lookups.

### Acceptance Criteria

- API authorization decisions reflect the same chain semantics as membership store tests.

---

## Finding 5: Memory fan-out uses scope-tree ancestry but not principal membership

Risk: Medium  
Impact: Retrieval scope fan-out may diverge from principal membership policy.

### Tasks

- [x] Decide policy explicitly:
  - Option A: keep scope-tree fan-out for retrieval, enforce principal auth at boundary only.
  - Option B: combine scope-tree fan-out with principal effective scope intersection.
- [x] Implement chosen policy in memory recall path:
  - Chosen: Option B (intersection).
  - Implemented: `fanOutScopes ∩ authorizedScopes` in `memory.Recall`.
  - Wired authorized scope IDs into memory recall call sites (REST + MCP).
- [x] Add tests for chain and unrelated-branch cases verifying returned results are authz-safe.
  - Added unit tests in `internal/memory/recall_test.go`:
    - `TestRecall_IntersectAuthorizedScopeIDs`
    - `TestRecall_EmptyIntersectionSkipsDBQueries`
  - Added REST integration regression:
    - `TestREST_Recall_IntersectsFanOutWithPrincipalScopes`
    - verifies ancestor-scope memory is not leaked when principal effective scopes do not include the ancestor.

### Acceptance Criteria

- Recall never returns memories outside authorized scope set.

---

## Finding 6: Current multi-hop tests validate membership store, not end-to-end API

Risk: Info  
Impact: Good unit/integration coverage for store layer, limited API security assurance.

### Tasks

- [x] Add API-level security integration suite:
  - `internal/api/rest/scope_authz_integration_test.go`
  - `internal/api/mcp/scope_authz_integration_test.go`
- [x] Build reusable fixture graph:
  - principals + scopes + memberships for chains and unrelated branch
  - Added `internal/testhelper.CreateScopeAuthzGraph(...)`.
- [x] Assert both positive and negative cases for each scope-taking route/tool.
  - Added REST chain matrix test:
    - `TestREST_ScopeAuthz_WriteEndpoints_MultiHopChainMatrix`
  - Added MCP chain matrix test:
    - `TestMCP_ScopeAuthz_MultiHopChainMatrix`
  - Existing endpoint/tool scope-authz suites retained and expanded from Findings 2–5.

### Acceptance Criteria

- End-to-end tests fail if any endpoint/tool omits scope authorization.

---

## Cross-Cutting Tasks

- [x] Add a route/tool inventory in code comments or test table listing every scope-taking operation.
  - Added explicit inventory tables in:
    - `internal/api/rest/scopeauth_inventory_test.go` (`restScopeRouteInventory`)
    - `internal/api/mcp/scopeauth_inventory_test.go` (`mcpScopeToolInventory`)
- [x] Add CI gate for scope authz tests (unit + integration tags).
  - Added GitHub Actions job `scope-authz` in `.github/workflows/ci.yml`:
    - unit: `go test ./internal/api/scopeauth ./internal/memory`
    - integration: `go test -tags integration ./internal/api/rest ./internal/api/mcp -run "Test(REST|MCP)_ScopeAuthz_|TestREST_Recall_IntersectsFanOutWithPrincipalScopes"`
- [x] Add logging fields for denied scope attempts:
  - principal_id
  - requested_scope_id
  - token_id
  - endpoint/tool
  - Implemented for both REST and MCP deny paths.
- [x] Add metrics counter:
  - `postbrain_scope_authz_denied_total{surface="rest|mcp", endpoint="..."}`
  - Added `metrics.ScopeAuthzDenied` and hooked increments in REST/MCP deny helpers.

---

## TDD Rule (Mandatory)

- [x] For every completed item below, write failing unit tests first (`red`) before implementation (`green`) and refactor.
- [ ] Do not implement authorization logic before corresponding unit tests exist.

## Recommended Implementation Order

1. Shared authorization helper unit tests + helper implementation (Finding 1).
2. Per-endpoint/tool unit tests first, then protect scope-taking writes (Finding 2).
3. Per-endpoint unit tests first, then protect ID-based reads/writes (Finding 3).
4. Unit tests first, then integrate multi-hop effective scopes in helper (Finding 4).
5. Unit tests first, then align memory fan-out policy with authz model (Finding 5).
6. Add end-to-end security integration suite and CI gate (Finding 6).

---

## Definition of Done

- Every scope-taking API/tool path has explicit authorization checks.
- Token scope restrictions and multi-hop principal chain restrictions are both enforced.
- ID-based endpoints enforce object scope authorization.
- Integration tests cover allowed/denied behavior for chain and unrelated branches.
- `go test ./...`, `make test-integration`, and `make lint` all pass.
