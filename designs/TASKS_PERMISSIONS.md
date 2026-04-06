# Postbrain — Permissions Implementation Tasks

All tasks follow strict TDD: failing test written first, then implementation to make it pass, then refactor. See `designs/DESIGN_PERMISSIONS.md` for the full specification.

---

## Sequencing

**Phase 1** builds the `internal/authz` package as a pure-logic, database-free single source of truth for all permission rules. It must be complete with a full unit test suite before any later phase begins.

**Phase 2** builds the database layer on top of Phase 1: migrations, queries, and a DB-backed effective-permission resolver with integration tests.

**Phase 3** updates the three existing surfaces (REST, MCP, WebUI) and the OAuth layer. For each surface: existing tests are updated or extended first, then the implementation is replaced. No surface implementation changes without a test covering the new behaviour first.

---

## Phase 1 — `internal/authz` package: permission API and unit tests

### 1.1 — Permission constants, resource registry, and shorthand expansion

- [ ] `internal/authz/permissions_test.go` — test that every `Resource` constant is defined; test that every `Operation` constant is defined; test that `ValidOperations(resource)` returns exactly the operations listed in the design for each resource; test that `Expand("read")` returns all `:read` permissions across all resources; test that `Expand("write")` expands likewise; test that `Expand("memories:read")` returns only `memories:read`; test that `Expand` of an unknown permission returns an error
- [ ] `internal/authz/permissions.go` — `Resource` typed string constants (`memories`, `knowledge`, `collections`, `skills`, `sessions`, `graph`, `scopes`, `principals`, `tokens`, `sharing`, `promotions`); `Operation` typed string constants (`read`, `write`, `edit`, `delete`); `Permission` type as `"{resource}:{operation}"`; `ValidOperations(Resource) []Operation` registry; `Expand(raw string) ([]Permission, error)` — expands shorthand bare operations into full resource:operation pairs; `AllPermissions() []Permission` — every valid permission

### 1.2 — Membership role definitions

- [ ] `internal/authz/roles_test.go` — test that `RolePermissions(RoleMember)` returns exactly the set defined in the design (no more, no less); same for `RoleAdmin` and `RoleOwner`; test that `RoleAdmin` is a strict superset of `RoleMember`; test that `RoleOwner` is a strict superset of `RoleAdmin`; test that `ParseRole("member")` / `"admin"` / `"owner"` parses correctly; test that unknown role returns error
- [ ] `internal/authz/roles.go` — `Role` typed string; `RoleMember`, `RoleAdmin`, `RoleOwner` constants; `ParseRole(string) (Role, error)`; `RolePermissions(Role) PermissionSet` — returns the canonical permission set for each membership role as specified in the design

### 1.3 — PermissionSet operations

- [ ] `internal/authz/permset_test.go` — test `NewPermissionSet` from a slice of raw strings (with shorthand expansion); test `Satisfies(Permission)` for exact match; test `Satisfies` for shorthand set satisfying specific resource:operation; test `Union` of two disjoint sets; test `Union` is idempotent on overlapping sets; test `Intersect` of two sets; test `Intersect` of disjoint sets yields empty; test `IsEmpty`; test `Contains`; test round-trip serialisation to `[]string`
- [ ] `internal/authz/permset.go` — `PermissionSet` type (backed by a set of `Permission`); `NewPermissionSet(raw []string) (PermissionSet, error)` (expands shorthand, validates each entry); `(PermissionSet) Satisfies(required Permission) bool`; `Union(sets ...PermissionSet) PermissionSet`; `Intersect(a, b PermissionSet) PermissionSet`; `(PermissionSet) IsEmpty() bool`; `(PermissionSet) Contains(Permission) bool`; `(PermissionSet) ToSlice() []string` (sorted, canonical)

### 1.4 — Token permission parsing and validation

- [ ] `internal/authz/token_test.go` — test that bare `read`/`write`/`edit`/`delete` are valid token permissions; test that `resource:operation` pairs are valid when the operation is in `ValidOperations` for that resource; test that `resource:operation` with an invalid operation for that resource is rejected (e.g. `graph:write`); test that `admin` is rejected (removed); test that an empty permissions slice is rejected; test `EffectiveTokenPermissions(principalPerms, tokenPerms PermissionSet) PermissionSet` returns the intersection; test that the intersection never grants permissions the principal does not hold; test that a fully-permissioned token with a restricted principal is still restricted
- [ ] `internal/authz/token.go` — `ParseTokenPermissions(raw []string) (PermissionSet, error)` — validates each entry against the resource registry and disallows legacy `admin`; `EffectiveTokenPermissions(principal, token PermissionSet) PermissionSet` — returns the intersection, enforcing the invariant that a token can never exceed principal permissions

### 1.5 — Promotion path logic

- [ ] `internal/authz/promotions_test.go` — test `PromotionAccess` when caller has both `memories:write` and `knowledge:write` → returns `PathDirect` and standard review count; test when caller has only `promotions:write` → returns `PathReview` and elevated review count; test when caller has neither → returns error / denied; test that the elevated review count is strictly greater than the standard count; test that the constants `StandardReviewRequired` and `ElevatedReviewRequired` match the values stated in the design
- [ ] `internal/authz/promotions.go` — `PromotionPathKind` type; `PathDirect`, `PathReview` constants; `StandardReviewRequired`, `ElevatedReviewRequired` int constants; `PromotionAccess(callerPermissions PermissionSet) (PromotionPathKind, int, error)` — determines which path applies and what `review_required` value to set on the resulting `promotion_request`

### 1.6 — Scope inheritance helpers (pure logic)

- [ ] `internal/authz/inheritance_test.go` — test `ApplyUpwardRead(grants map[scopeID]PermissionSet, ancestors []scopeID)` — given a read grant on a scope, verify that the same `:read` permissions are added for each ancestor; test that non-read permissions are not propagated upward; test that downward inheritance is implied (grants on a parent apply to children — consumers look up the tree); test `MergeGrants(grants ...map[scopeID]PermissionSet)` produces the additive union per scope
- [ ] `internal/authz/inheritance.go` — `ApplyUpwardRead(grants map[ScopeID]PermissionSet, ancestors []ScopeID) map[ScopeID]PermissionSet`; `MergeGrants(sources ...map[ScopeID]PermissionSet) map[ScopeID]PermissionSet`; `ScopeID` type alias for `uuid.UUID`; note: downward inheritance is handled at query time (ancestor lookup in SQL), not in this layer

---

## Phase 2 — Database layer: migrations, queries, and resolver

### 2.1 — Schema migrations

- [ ] `internal/db/migrations/000015_permissions.up.sql` — `ALTER TABLE principals ADD COLUMN is_system_admin BOOLEAN NOT NULL DEFAULT false`; `CREATE TABLE scope_grants (...)` as specified in the design; add `scope_grants_principal_idx` and `scope_grants_scope_idx`
- [ ] `internal/db/migrations/000015_permissions.down.sql` — reverse the above
- [ ] `internal/db/migrations/000016_token_permissions_v2.up.sql` — data migration: `UPDATE tokens SET permissions = ARRAY['read','write','edit','delete'] WHERE 'admin' = ANY(permissions)`; add `CHECK` constraint disallowing `admin` in `tokens.permissions`
- [ ] `internal/db/migrations/000016_token_permissions_v2.down.sql` — drop the check constraint; optionally reverse data migration
- [ ] `internal/db/migrations/migration_test.go` (or extend existing) — test that after applying 000015 the `scope_grants` table and `principals.is_system_admin` column exist; test that after applying 000016 no token has `admin` in permissions; test that attempting to insert `admin` into `tokens.permissions` fails the check constraint

### 2.2 — sqlc queries for `scope_grants`

- [ ] `internal/db/scope_grants.sql` — `CreateScopeGrant`, `GetScopeGrant (principal_id, scope_id)`, `ListScopeGrantsByPrincipal`, `ListScopeGrantsByScope`, `UpdateScopeGrantPermissions`, `DeleteScopeGrant`, `DeleteExpiredScopeGrants`
- [ ] `internal/db/scope_grants.sql.go` — generated by `sqlc generate` (run after writing the SQL file)
- [ ] `internal/db/scope_grants_test.go` — integration tests for each query: create, list by principal, list by scope, update, delete, expired grant not returned

### 2.3 — DB-backed effective permission resolver

- [ ] `internal/authz/resolver_test.go` — integration tests (build tag `integration`, require a real DB); test resolver returns full permissions for a `systemadmin` principal; test resolver returns ownership permissions when `principal_id = scope.principal_id`; test resolver returns `member` role permissions when principal is a `member` of the owning principal; test resolver returns `admin` role permissions; test resolver returns `owner` role permissions; test resolver returns union when principal has both membership and a direct grant; test direct scope grant permissions are returned; test downward inheritance: grant on parent scope → child scope gets same permissions; test upward read: grant on child scope → ancestor scopes get matching `:read` permissions; test upward read does not propagate `write`, `edit`, or `delete` to ancestors; test that expired scope grants are not included; test `systemadmin` flag on principal bypasses all other checks
- [ ] `internal/authz/resolver.go` — `Resolver` interface: `EffectivePermissions(ctx, principalID, scopeID) (PermissionSet, error)` and `HasPermission(ctx, principalID, scopeID, permission Permission) (bool, error)`; `DBResolver` struct implementing `Resolver` with a `pgxpool.Pool`; resolution algorithm as specified in the design (systemadmin → ownership → membership → direct grants → union → upward read)
- [ ] `internal/authz/resolver_cache.go` — `CachedResolver` wrapping `Resolver` with a short-lived in-process cache (keyed on `(principalID, scopeID)`) to avoid repeated DB round-trips per request; configurable TTL; cache invalidation on scope grant mutation

### 2.4 — Effective token permission resolver

- [ ] `internal/authz/token_resolver_test.go` — integration tests; test that token with `["read"]` against a `member` principal yields all `:read` permissions the member holds; test that token with `["memories:read"]` against an `owner` principal yields only `memories:read`; test that token `scope_ids` restriction excludes scopes not in the declared list; test that token `scope_ids` restriction still includes descendants of declared scopes; test that token cannot surface permissions the principal does not hold (intersection invariant); test that a revoked or expired token returns no permissions
- [ ] `internal/authz/token_resolver.go` — `TokenResolver` wrapping `Resolver`; `EffectiveTokenPermissions(ctx, token *db.Token) (map[ScopeID]PermissionSet, error)` — applies scope restriction and permission intersection; `HasTokenPermission(ctx, token *db.Token, scopeID ScopeID, permission Permission) (bool, error)`

---

## Phase 3 — Surface updates: REST, MCP, WebUI, OAuth

For each surface: update or add failing tests first, then update the implementation. No implementation change without a test covering the new behaviour.

### 3.1 — OAuth scopes alignment

- [ ] `internal/oauth/scopes_test.go` — update/extend: test that `ParseScopes` accepts all new `{resource}:{operation}` values; test that `ParseScopes` rejects `admin`; test that `ScopeToPermissions` produces valid `authz.PermissionSet` values; test all new resource:operation combinations that are valid OAuth scopes
- [ ] `internal/oauth/scopes.go` — replace hardcoded scope constants with the full `{resource}:{operation}` set from `internal/authz`; remove `ScopeAdmin`; update `knownScopes` registry; update `ScopeToPermissions` to use `authz.ParseTokenPermissions`

### 3.2 — REST API: per-route permission enforcement

- [ ] `internal/api/rest/permission_authz_integration_test.go` — extend existing tests to cover: each route in the permission matrix from the design; verify `403` when token lacks the specific `{resource}:{operation}` for that route; verify `200`/`201` when token has the exact required permission; verify `systemadmin` token can access all routes; verify scope-restricted token is denied access to out-of-scope resources; cover the full matrix (all endpoints × all relevant permission cases)
- [ ] `internal/api/rest/permissionauth.go` — replace the HTTP-method heuristic (`GET=read, else write`) with a route-specific permission table mapping each `(method, path pattern)` to its required `authz.Permission`; integrate `authz.TokenResolver` for the check; return `403` with a consistent error body on denial
- [ ] `internal/api/rest/router.go` — wire `authz.Resolver` and `authz.TokenResolver` into the middleware chain; remove dependency on the old `auth.HasReadPermission` / `auth.HasWritePermission` helpers

### 3.3 — REST API: scope authorization update

- [ ] `internal/api/rest/scopeauth_integration_test.go` — extend/add tests: verify that a principal with only a direct scope grant on scope S can access scope S; verify that a principal with membership in an ancestor-owning principal can access child scopes; verify that a token with `scope_ids` restricted to S cannot access sibling scope T even if the principal can; verify upward-read: a principal with access to child scope sees parent scope in `GET /v1/scopes`
- [ ] `internal/api/scopeauth/scopeauth.go` — replace `EffectiveScopeIDs` (which only computed reachable scope IDs) with `authz.Resolver`-backed checks; `AuthorizeContextScope(ctx, scopeID, permission)` now calls `HasTokenPermission`; remove the old two-stage scope check

### 3.4 — MCP: tool permission assignments

- [ ] `internal/api/mcp/permission_authz_integration_test.go` — extend existing tests to cover: each tool in the MCP permission matrix from the design; verify that `forget` requires `memories:delete`; verify that `skill_invoke` and `skill_install` require only `skills:read`; verify that `promote` succeeds with `memories:write + knowledge:write` and also with `promotions:write`; verify that `graph_query` requires `graph:read`
- [ ] `internal/api/mcp/permissionauth.go` — update `permissionRead` / `permissionWrite` constants to use `authz.Permission` values; `withToolPermission` accepts `authz.Permission`; integrate `authz.TokenResolver`
- [ ] `internal/api/mcp/server.go` — update every `withToolPermission(...)` call to the specific `authz.Permission` from the design's MCP tool matrix

### 3.5 — WebUI: token management

- [ ] `internal/ui/permission_authz_integration_test.go` — update existing tests; add tests for: token creation form accepts all new `{resource}:{operation}` values; token creation form rejects `admin`; a `tokens:read`-only token can list tokens (self-service); a token without `principals:edit` cannot manage another principal's tokens via the UI; `systemadmin` principal can manage any token
- [ ] `internal/ui/tokens.go` — update `parseTokenPermissions` to use `authz.ParseTokenPermissions`; remove `admin` from allowed values; update token creation form handler; update scope editing and revocation to use `authz.TokenResolver` for authorization check
- [ ] `internal/ui/web/templates/tokens.html` — update permission checkboxes to reflect new resource-scoped permissions; replace `admin` checkbox; group by resource for clarity

### 3.6 — Auth middleware: token validation update

- [ ] `internal/auth/middleware_test.go` — update/add tests: verify that a token with legacy `admin` permission (still in DB pre-migration) is handled gracefully; verify that a token's permissions are parsed via `authz.ParseTokenPermissions`; verify that `ContextKeyPermissions` now stores `authz.PermissionSet` rather than `[]string`
- [ ] `internal/auth/middleware.go` — update bearer token middleware to parse `token.Permissions` via `authz.ParseTokenPermissions`; inject `authz.PermissionSet` into context rather than raw `[]string`; inject `authz.TokenResolver` for downstream use

### 3.7 — Remove old permission helpers

- [ ] `internal/auth/permissions_test.go` — delete (replaced by `internal/authz` tests)
- [ ] `internal/auth/permissions.go` — delete `HasReadPermission`, `HasWritePermission`, `permissionCapabilities`, old constants; retain only what is needed for backward-compatible token lookup during migration window; add deprecation note directing to `internal/authz`

---

## Phase 4 — Scope grants REST API

### 4.1 — REST endpoints for `scope_grants`

- [ ] `internal/api/rest/scope_grants_test.go` — test `POST /v1/scopes/{id}/grants`: creates grant, requires `sharing:write` on the scope, rejects escalation beyond caller's own permissions; test `GET /v1/scopes/{id}/grants`: lists grants, requires `sharing:read`; test `DELETE /v1/scopes/{id}/grants/{grant_id}`: revokes, requires `sharing:delete`; test that an expired grant is excluded from list; test that a grant from a `systemadmin` caller is accepted
- [ ] `internal/api/rest/scope_grants.go` — `handleCreateScopeGrant`, `handleListScopeGrants`, `handleDeleteScopeGrant`; uses `authz.Resolver` to verify caller permissions before mutating; enforces anti-escalation: caller cannot grant permissions they do not hold
- [ ] `internal/api/rest/router.go` — add `POST /v1/scopes/{id}/grants`, `GET /v1/scopes/{id}/grants`, `DELETE /v1/scopes/{id}/grants/{grant_id}` routes