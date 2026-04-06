# Postbrain — Implementation Task List

## Design & Documentation (COMPLETE)

- [x] Three-layer model design (Memory, Knowledge, Skills)
- [x] Principal and scope hierarchy model
- [x] Database schema (all tables, indexes, triggers, pg_cron jobs)
- [x] MCP tool specifications (remember, recall, forget, context, summarize, publish, endorse, promote, collect, skill_search, skill_install, skill_invoke)
- [x] REST API specification (all endpoints, pagination)
- [x] Hybrid retrieval strategy (HNSW ANN + BM25 FTS + pg_trgm trigram similarity + scoring formula)
- [x] Knowledge visibility and sharing model
- [x] Promotion workflow (memory → knowledge → skill)
- [x] Staleness detection (3 signals: source_modified, contradiction_detected, low_access_age)
- [x] Schema migration strategy (golang-migrate, advisory locks, zero-downtime)
- [x] Authentication design (tokens table, SHA-256 hashing, scope enforcement)
- [x] Apache AGE optional graph overlay design
- [x] Deployment guide (Docker Compose, production considerations)
- [x] Configuration schema (all keys documented)
- [x] postbrain-hook CLI specification (snapshot, summarize-session, skill sync/install/list)
- [x] Knowledge artifact state machine (draft → in_review → published → deprecated)
- [x] Authorization rules for knowledge write operations
- [x] Background job specifications (reembed, consolidation, contradiction detection)

---

## Implementation Tasks

### Permissions Redesign

- [x] 2026-04-06: `internal/authz` package bootstrapped — Phase 1.1 complete (see `designs/TASKS_PERMISSIONS.md`):
  - `internal/authz/permissions.go`: `Resource`, `Operation`, `Permission` types; `ValidOperations`, `AllResources`, `AllPermissions`, `Expand`
  - `internal/authz/permissions_test.go`: full unit test coverage for constants, valid operations per resource, shorthand expansion, resource-scoped expansion, error cases

### Maintenance

- [x] 2026-04-06: Fixed token scope editor options being incorrectly limited by current session token scope filter (TDD-first):
  - Added `internal/ui/tokens_integration_test.go::TestTokensPage_EditScopes_ShowsPrincipalEffectiveScopes`:
    - reproduces scoped-session scenario where edited token has broader scope assignments.
  - Updated token page scope option source in `internal/ui/tokens.go`:
    - token create/edit scope options now use principal-effective writable scopes, not current session token-restricted scope set.
  - Added helper in `internal/ui/handler.go`:
    - `effectivePrincipalScopesForRequest` to resolve principal writable scopes without applying current token `scope_ids` restrictions.

- [x] 2026-04-05: Updated integration test expectations for tightened authz behavior:
  - Updated `internal/ui/principals_integration_test.go`:
    - non-admin principal mutation attempt now expects direct `403` (route-level admin gating) instead of `200` with rendered form error.
  - Updated `internal/api/rest/rest_integration_test.go` scope CRUD expectation:
    - after transferring scope ownership, delete by the original owner now expects `403` (owner/admin requirement) instead of `204`.

- [x] 2026-04-05: Hid principal-management UI sections for non-admin users (TDD-first):
  - Added principal-admin gating in `internal/ui/handler.go` for principal-management routes:
    - `/ui/principals`
    - `/ui/principals/{id}`
    - `/ui/memberships`
    - `/ui/memberships/delete`
    - non-admin requests now receive `403 forbidden`.
  - Updated base layout in `internal/ui/web/templates/base.html`:
    - "Principals" sidebar entry is now rendered only for users with any principal admin role.
  - Added integration coverage in `internal/ui/principals_integration_test.go`:
    - `TestPrincipalsPage_RequiresAdminRole`
    - `TestSidebar_HidesPrincipalsForNonAdmin`
    - `TestSidebar_ShowsPrincipalsForAdmin`.

- [x] 2026-04-05: Enforced token permissions in WebUI and surfaced token permissions in UI flows (TDD-first):
  - Added centralized WebUI permission gating in `internal/ui/handler.go`:
    - authenticated UI GET/HEAD/OPTIONS routes require token read permission
    - authenticated mutating routes require token write permission
    - returns `403 forbidden: insufficient permissions` for denied operations.
  - Updated WebUI token creation in `internal/ui/tokens.go`:
    - parses and validates selected permissions from the create form (`read`, `write`, `admin`)
    - persists selected permissions on token creation instead of always relying on DB defaults
    - defaults to `read,write` when no permission values are supplied.
  - Updated token management template in `internal/ui/web/templates/tokens.html`:
    - added permissions selection to the token creation form
    - added permissions column in token list to reflect stored `tokens.permissions`.
  - Added integration coverage:
    - `internal/ui/permission_authz_integration_test.go::TestUI_PermissionAuthz_ReadVsWrite`
    - `internal/ui/tokens_integration_test.go::TestCreateToken_UsesSelectedPermissions`
    - `internal/ui/tokens_integration_test.go::TestTokensPage_ShowsTokenPermissions`.

- [x] 2026-04-05: Enforced token permissions for MCP tools (TDD-first):
  - Added MCP permission middleware helpers in `internal/api/mcp/permissionauth.go`:
    - `withToolPermission` tool wrapper with `read`/`write` permission modes
    - standardized permission denial response (`forbidden: insufficient permissions`).
  - Wrapped MCP tool registrations in `internal/api/mcp/server.go` with permission requirements:
    - read tools require read permission (`recall`, `context`, `graph_query`, `skill_search`, `knowledge_detail`, `list_scopes`, `collect`)
    - mutating tools require write permission (`remember`, `forget`, `summarize`, `publish`, `endorse`, `promote`, `skill_install`, `skill_invoke`, `session_begin`, `session_end`, `synthesize_topic`).
  - Added mixed-action enforcement for `collect` in `internal/api/mcp/collect.go`:
    - `create_collection` and `add_to_collection` require write permission.
  - Added integration regression coverage:
    - `internal/api/mcp/permission_authz_integration_test.go::TestMCP_PermissionAuthz_ReadVsWrite`.
  - Updated MCP integration test auth contexts to include explicit token permissions for compatibility with permission enforcement:
    - `internal/api/mcp/mcp_integration_test.go`
    - `internal/api/mcp/scope_authz_integration_test.go`
    - `internal/api/mcp/list_scopes_integration_test.go`.

- [x] 2026-04-05: Enforced token permissions for REST API requests (TDD-first):
  - Added shared permission evaluation helpers in `internal/auth/permissions.go`:
    - `HasReadPermission`
    - `HasWritePermission`
    - supports generic (`read`, `write`, `admin`) and OAuth-style (`*:read`, `*:write`) permissions.
  - Added REST permission middleware in `internal/api/rest/permissionauth.go` and wired it in `internal/api/rest/router.go`:
    - GET/HEAD/OPTIONS require read permission
    - mutating methods require write permission.
  - Added integration coverage:
    - `internal/api/rest/permission_authz_integration_test.go::TestREST_PermissionAuthz_ReadVsWrite`.
  - Updated default token permission bootstrap in `internal/db/compat.go`:
    - tokens created without explicit permissions now default to `["read", "write"]` to preserve existing behavior while enabling enforcement.
  - Added unit coverage:
    - `internal/auth/permissions_test.go`.

- [x] 2026-04-05: Added WebUI support to edit token scope restrictions (TDD-first):
  - Added token scope update endpoint in UI:
    - `POST /ui/tokens/{id}/scopes` (`internal/ui/tokens.go`)
    - validates ownership and active token status before updating `scope_ids`.
  - Added DB helper:
    - `db.UpdateTokenScopes` in `internal/db/compat.go` for principal-owned token scope updates.
  - Reused owned-token lookup logic in UI token handlers to keep ownership checks consistent.
  - Added token scope edit controls to token management page:
    - per-token “Edit scopes” form with scope checkboxes preselected from current `scope_ids`
    - implemented via template helper `tokenHasScope`.
  - Added integration coverage:
    - `internal/ui/tokens_integration_test.go::TestUpdateTokenScopes_OwnToken_UpdatesScopeIDs`
    - `internal/ui/tokens_integration_test.go::TestUpdateTokenScopes_OtherPrincipalToken_ReturnsForbidden`.
  - Updated token scope editing UX to use modal dialog pattern (consistent with other WebUI admin flows) instead of inline `<details>`.
  - Added integration regression:
    - `internal/ui/tokens_integration_test.go::TestTokensPage_UsesDialogForScopeEditing`.

- [x] 2026-04-05: Restricted scope delete actions in WebUI to scope owners/admins:
  - Kept server-side delete authorization as owner/admin-only (`handleDeleteScope` + `hasScopeAdminAccess`).
  - Updated scopes page rendering in `internal/ui/handler.go` to compute per-scope management capability (`CanManage`/`CanDelete`) and pass it to the template.
  - Updated `internal/ui/web/templates/scopes.html` to fully hide owner/repo/sync/delete controls/actions for non-owner/non-admin users.
  - Added integration regressions in `internal/ui/scopes_integration_test.go` to assert non-admin members:
    - do not receive delete action links for parent scopes
    - do not receive owner change action controls
    - cannot change owner of parent scopes via direct POST.

- [x] 2026-04-05: Fixed scoped UI dropdown hierarchy to include parent scopes (TDD-first):
  - Updated `internal/ui/handler.go` `authorizedScopesForRequest` token filtering:
    - expands token `scope_ids` to include each scope’s ancestor scopes via `db.GetAncestorScopeIDs`
    - keeps principal-effective-scope intersection intact.
  - This restores parent-scope visibility for scoped session tokens across shared UI selectors (`/ui/memories`, `/ui/query`, `/ui/graph`, `/ui/graph3d`, and other pages using the shared resolver).
  - Added integration regression coverage:
    - `internal/ui/scopes_integration_test.go::TestScopedSessionToken_IncludesParentScopesInDropdowns`.

- [x] 2026-04-05: Restricted principal mutations to admin-only across REST and WebUI (TDD-first):
  - Added principal-admin resolution helpers in `internal/principals/membership.go`:
    - `IsPrincipalAdmin` (admin on target principal or any ancestor principal)
    - `HasAnyAdminRole` (for admin-gated principal creation).
  - Enforced admin checks for REST principal mutation endpoints in `internal/api/rest/orgs.go`:
    - `POST /v1/principals`
    - `PUT /v1/principals/{id}`
    - `DELETE /v1/principals/{id}`
    - `POST /v1/principals/{id}/members`
    - `DELETE /v1/principals/{id}/members/{member_id}`.
  - Enforced admin checks for WebUI principal mutation handlers in `internal/ui/handler.go`:
    - `handleCreatePrincipal`
    - `handleUpdatePrincipal` (slug/display_name changes)
    - `handleAddMembership`
    - `handleDeleteMembership`.
  - Added integration regression coverage:
    - `internal/api/rest/principal_admin_authz_integration_test.go`
    - `internal/ui/principals_integration_test.go::TestPrincipalsPage_PrincipalSlugChangeRequiresAdmin`.
  - Updated existing WebUI principal update integration test to run with authenticated admin context:
    - `internal/ui/handler_principals_integration_test.go::TestHandleUpdatePrincipal_Success`.

- [x] 2026-04-05: Enforced scope-admin and delete semantics across WebUI, REST, and MCP (TDD-first):
  - Added REST scope-admin authorization for scope mutations in `internal/api/rest/scopes.go`:
    - create child scope requires admin on parent scope
    - update scope, update owner, delete scope, set repo, and sync repo require scope admin.
  - Added REST delete-specific authorization in `internal/api/rest/scopeauth.go` and applied to:
    - `DELETE /v1/memories/{id}` (`internal/api/rest/memories.go`)
    - `DELETE /v1/collections/{id}/items/{artifact_id}` (`internal/api/rest/collections.go`)
    - rule: member can write parent scopes, but delete only in caller-owned scopes (not ancestor scopes).
  - Added MCP delete-specific authorization:
    - `forget` now resolves memory scope and enforces delete scope policy before soft/hard delete (`internal/api/mcp/forget.go`, `internal/api/mcp/scopeauth.go`).
  - Added WebUI scope-admin enforcement in `internal/ui/handler.go`:
    - delete scope, set scope owner, set repo, sync repo, and create child scope (with `parent_id`) now require scope admin.
  - Added integration regression tests:
    - `internal/api/rest/scope_authz_integration_test.go::TestREST_ScopeAuthz_WriteParentAllowed_DeleteParentDenied`
    - `internal/api/rest/scope_admin_authz_integration_test.go::TestREST_ScopeAdminAuthz_MemberCannotAdminParentScope`
    - `internal/api/mcp/scope_authz_integration_test.go::TestMCP_ScopeAuthz_ForgetWriteParentAllowedDeleteParentDenied`
    - `internal/ui/scopes_integration_test.go::TestScopesPage_MemberCannotAdminParentScope`.

- [x] 2026-04-05: Unified UI scope filtering through one shared authorization function:
  - Added `authorizedScopesForRequest` in `internal/ui/handler.go` as the single scope-filtering function (effective principal scopes intersected with token `scope_ids` restrictions).
  - Replaced per-page direct scope listing with this shared function for:
    - memories
    - query playground
    - artifacts
    - promotions
    - graph 2D/3D
    - scope-bound create forms (`knowledge_new`, `collections_new`)
    - token scope picker.
  - Added scoped data filtering for pages without a scope selector:
    - collections
    - staleness
    - skills.
  - Added integration regression coverage in `internal/ui/page_scope_filtering_integration_test.go` to verify hidden scopes/data do not leak across all listed pages.

- [x] 2026-04-05: Fixed scoped-session fallback in shared UI scope filter:
  - Updated `authorizedScopesForRequest` in `internal/ui/handler.go` to fall back to explicit token `scope_ids` when principal effective-scope resolution is empty.
  - This restores visibility for scoped session tokens while keeping filtering centralized in a single function.

- [x] 2026-04-05: Restricted UI principals page to reachable principals:
  - Updated `internal/ui/handler.go` `renderPrincipals` to filter principals and membership rows to the authenticated principal’s reachable principal set (self + ancestor membership chain).
  - Added helper `reachablePrincipalIDSet` using `db.GetAllParentIDs`.
  - Added integration regression coverage in `internal/ui/principals_integration_test.go` to ensure unrelated principals are not visible.

- [x] 2026-04-05: Added Web UI logout action and session invalidation flow:
  - Added `POST /ui/logout` handler in `internal/ui/auth.go` to clear `pb_session` cookie and redirect to `/ui/login`.
  - Wired logout route in `internal/ui/handler.go`.
  - Added logout button to the application sidebar in `internal/ui/web/templates/base.html`.
  - Added coverage:
    - unit: `internal/ui/handler_auth_test.go::TestLogoutPOST_ClearsCookieAndRedirects`
    - integration: `internal/ui/auth_integration_test.go::TestLogoutPOST_ClearsSessionAndRequiresLoginAgain`.

- [x] 2026-04-05: Restricted MCP `list_scopes` to authorized writable scopes:
  - Updated `internal/api/mcp/list_scopes.go` to resolve scopes from the caller’s authorized scope set instead of returning global scope inventory.
  - Added MCP scope-authorization helper in `internal/api/mcp/scopeauth.go` to intersect effective principal scopes with token scope restrictions.
  - Added integration coverage in `internal/api/mcp/list_scopes_integration_test.go` to verify non-writable scopes are excluded from MCP list responses.

- [x] 2026-04-05: Restricted REST `/v1/scopes` listing to authorized writable scopes:
  - Updated `internal/api/rest/scopes.go` to return only scopes in the caller’s authorized scope set (effective principal scopes intersected with token scope restrictions).
  - Added REST scope-authorization helper in `internal/api/rest/scopeauth.go` for reusable authorized scope resolution.
  - Added integration coverage in `internal/api/rest/scopes_list_authz_integration_test.go` to verify non-writable scopes are excluded from list results.

- [x] 2026-04-05: Restricted UI scopes page to writable scopes for current principal:
  - Updated `internal/ui/handler.go` `renderScopes` to filter listed scopes by the authenticated principal’s effective writable scope set.
  - Added helper `writableScopeIDSet` to resolve effective scope IDs via membership graph.
  - Added integration regression coverage in `internal/ui/scopes_integration_test.go` to ensure `/ui/scopes` does not expose non-writable scopes.

- [x] 2026-04-05: Restricted UI token management to token owners:
  - Updated `internal/ui/tokens.go` so `/ui/tokens` lists only tokens for the currently authenticated principal.
  - Added ownership enforcement in `handleRevokeToken` to deny revoking tokens owned by other principals (`403 forbidden`).
  - Added integration regression coverage in `internal/ui/tokens_integration_test.go` for:
    - per-principal token list visibility
    - cross-principal revoke denial.

- [x] 2026-04-05: Updated Helm chart embedding config to provider profiles:
  - Replaced legacy single-provider keys (`backend`, `ollama_url`, `openai_api_key`, model fields) in chart values/template with `config.embedding.providers`.
  - Updated `deploy/helm/postbrain/templates/_helpers.tpl` to render runtime `config.yaml` using `embedding.providers.<name>` entries.
  - Updated default chart values to define `embedding.providers.default` with the previous Ollama defaults.

- [x] 2026-04-05: Fixed docs layout image overflow in Web UI documentation pages:
  - Updated `site/src/layouts/DocsLayout.astro` markdown image styling to enforce responsive sizing (`max-width: 100%`, `height: auto`) and prevent oversized screenshots from overflowing the content column.

- [x] 2026-04-05: Added screenshot-based Web UI documentation and pages-safe image rewriting:
  - Added `docs/webui-guide.md` with detailed, user-oriented walkthroughs for:
    - principals/memberships
    - scopes/hierarchy
    - token management
    - memory list/detail workflows
    - knowledge upload/artifact workflows
    - entity graph 3D usage.
  - Linked the new guide from `docs/README.md`.
  - Added Web UI screenshot assets under `site/public/assets/images/`.
  - Updated `site/scripts/sync-docs.mjs` to rewrite docs image links during site sync so:
    - source markdown remains GitHub-friendly
    - generated docs pages use correct `/assets/...` (or `/<repo>/assets/...` on GitHub Pages) paths.

- [x] 2026-04-05: Added startup configuration/service visibility logging:
  - Extended `cmd/postbrain/main.go` startup logs to include:
    - config summary (`config_path`, server/db sizing, embedding batch/timeout)
    - enabled background jobs list
    - enabled OAuth providers list
    - embedding provider profile details (`name`, `backend`, `service_url`, model slugs, `has_api_key` without exposing secrets)
    - explicit service initialization markers (embedding service/factory, API surface + scheduler).
    - per-service startup confirmation logs for each created component (`db_pool`, embedding services, OAuth stores/server, MCP, REST, UI, metrics handler, listener, scheduler).
  - Added unit coverage in `cmd/postbrain/main_test.go` for:
    - `enabledJobNames`
    - `embeddingProviderInfos`
    - `enabledOAuthProviderNames`.
    - `startupServiceStepNames`.

- [x] 2026-04-05: Added high-dimension ANN support using halfvec expression indexing (TDD-first):
  - Kept per-model embedding table storage in full precision `vector(dims)` for all models.
  - Updated `internal/db/embedding_tables.go` provisioning logic:
    - for `dims <= 2000`: regular HNSW index on `embedding vector_cosine_ops`
    - for `dims > 2000`: HNSW expression index on `(embedding::halfvec(dims)) halfvec_cosine_ops`.
  - Updated `internal/db/embedding_repository.go` ANN query path:
    - for high-dimension models, similarity/order-by uses `embedding::halfvec(dims) <=> $1::halfvec` to match index expression.
  - Added unit coverage:
    - `internal/db/embedding_tables_test.go` (`embeddingStorageForDimensions`)
    - `internal/db/embedding_repository_test.go` (`similarityDistanceExpr`).
  - Added integration coverage:
    - `internal/db/embedding_tables_integration_test.go::TestEnsureEmbeddingModelTable_UsesHalfvecForHighDimensions` (column remains `vector(2560)`, index uses `halfvec_cosine_ops` expression).
  - Extended repository integration coverage:
    - `internal/db/embedding_repository_integration_test.go::TestEmbeddingRepository_QuerySimilar_HighDimensionHalfvecPath`
    - verifies high-dimension (`dims > 2000`) QuerySimilar succeeds end-to-end and returns expected nearest hits.
  - Follow-up test hardening:
    - relaxed `indexdef` assertion in `internal/db/embedding_tables_integration_test.go` to tolerate PostgreSQL expression normalization (`(embedding)::halfvec(...)`) while still asserting halfvec expression indexing intent.

- [x] 2026-04-05: Fixed re-embed text crash on NULL content rows (TDD-first):
  - Added integration regression test:
    - `internal/jobs/reembed_integration_test.go::TestReembedJob_RunText_NullContentMarksFailed`
  - Updated `internal/jobs/reembed.go`:
    - scan `content` as `sql.NullString` in `RunText` pending-row loop
    - treat NULL/blank content uniformly as failed-attempt rows via existing retry/failure logic
    - avoid hard job failure from `cannot scan NULL into *string`.

- [x] 2026-04-05: Moved embedding model administration from `postbrain-cli` to `postbrain` (TDD-first):
  - Added server-side `embedding-model` command group in `cmd/postbrain/embedding_model_cmd.go`:
    - `register`, `activate`, `list`
    - same provider-profile resolution behavior, including OpenAI default endpoint fallback when `service_url` is omitted.
  - Wired command into `postbrain` root command (`cmd/postbrain/main.go`) next to other operational commands.
  - Removed `embedding-model` command wiring and implementation from `cmd/postbrain-cli/main.go`.
  - Migrated CLI command coverage to `cmd/postbrain/embedding_model_cmd_test.go` and trimmed `cmd/postbrain-cli/main_test.go` to remaining responsibilities.
  - Updated operator docs to use `postbrain --config ... embedding-model ...`:
    - `docs/embedding-model-operations.md`
    - `docs/configuration.md`
    - `docs/troubleshooting-playbook.md`

- [x] 2026-04-05: Restored OpenAI default embedding endpoint behavior in CLI registration (TDD-first):
  - Added resolver regression test in `cmd/postbrain-cli/main_test.go` to assert `backend=openai` profiles default to `https://api.openai.com/v1` when `service_url` is omitted.
  - Updated `resolveProviderRegistrationFields` in `cmd/postbrain-cli/main.go` to apply the OpenAI default endpoint only for OpenAI profiles while keeping explicit `service_url` validation for other providers.

- [x] 2026-04-05: Simplified embedding model registration CLI to provider-config-only resolution (TDD-first):
  - Removed transient provider transport/model fields from CLI option flow in `cmd/postbrain-cli/main.go`.
  - `postbrain-cli embedding-model register` now derives registration metadata strictly from `embedding.providers.<provider-config>`:
    - `backend` -> DB `provider`
    - `service_url` -> DB `service_url`
    - `<content_type>_model` -> DB `provider_model`
  - Added validation that selected provider profile includes required `service_url`.
  - Updated CLI tests in `cmd/postbrain-cli/main_test.go`:
    - resolver tests now assert resolved `provider_config` + profile-derived fields
    - added regression test for missing profile `service_url`.

- [x] 2026-04-05: Addressed embedding/re-embed edge cases identified in PR review (TDD-first):
  - `internal/skills/store.go`:
    - `embedText` now treats nil service, nil embed result, and empty embedding vectors as errors.
    - added `ErrEmptyEmbedding` sentinel and unit test coverage in `internal/skills/store_test.go::TestCreate_EmptyEmbeddingReturnsError`.
    - prevents runtime vector insert failures caused by `pgvector.NewVector(nil)` on fixed-dimension columns.
  - `internal/jobs/reembed.go`:
    - `RunText` now re-embeds skills from the same text shape as write-path (`description + body`) using trimmed `concat_ws(...)`.
    - pending rows with empty/whitespace-only content are now moved through retry logic (`markEmbeddingFailedAttempt`) instead of being skipped forever.
  - Added integration coverage in `internal/jobs/reembed_integration_test.go`:
    - `TestReembedJob_RunText_SkillUsesDescriptionAndBody`
    - `TestReembedJob_RunText_EmptyContentMarksFailed`.

- [x] 2026-04-05: Added multi-provider embedding profile selection for model-driven embedding (TDD-first):
  - Extended runtime embedding config (`internal/config/config.go`) with:
    - `embedding.providers` map (`backend`, `service_url`, `openai_api_key`)
    - backward-compatible default profile synthesis only when no explicit provider map is configured.
  - Extended embedding model metadata resolution:
    - `internal/embedding/model_store.go` now reads `provider_config` from `embedding_models` (default fallback `default`).
    - `internal/embedding/factory.go` now resolves backend/service/auth via model `provider_config` profile override.
  - Added schema migration `000014_embedding_model_provider_config`:
    - adds `embedding_models.provider_config` (`TEXT NOT NULL DEFAULT 'default'`)
    - adds `embedding_models_provider_config_idx`.
  - Extended model registration and CLI:
    - `internal/db/RegisterEmbeddingModel` upsert now writes `provider_config`.
    - `postbrain-cli embedding-model register` now supports `--provider-config` (default `default`).
  - Added/updated tests:
    - config profile normalization tests (`internal/config/config_test.go`)
    - factory/store profile routing tests (`internal/embedding/factory_test.go`, `internal/embedding/model_store_test.go`)
    - CLI register profile flag tests (`cmd/postbrain-cli/main_test.go`)
    - integration test for default/override persistence (`internal/db/embedding_model_registration_integration_test.go`)
    - schema integration checks for `provider_config` column/index (`internal/db/embedding_schema_migration_integration_test.go`).
- [x] 2026-04-05: Tightened embedding config semantics to provider-scoped models and generic API key:
  - Removed config alias handling for `openai_api_key`; `api_key` is now the only supported key.
  - Removed top-level `EmbeddingConfig` provider transport fields (`backend`, `service_url`, `api_key`); runtime selection is now exclusively profile-based under `embedding.providers`.
  - Removed top-level embedding model fields from `EmbeddingConfig`; model slugs are now provider-profile scoped (`embedding.providers.<name>.text_model|code_model|summary_model`).
  - Updated startup embedding service construction to resolve text/code/summary model names from `providers.default`.
  - Updated OpenAI embed/summarize auth header wiring and related tests to use `api_key`.
  - Updated `config.example.yaml` and `docs/configuration.md` to document provider-scoped model slugs.

- [x] 2026-04-04: Integrated memory write-path dual-write to model tables (Step 7 partial, TDD-first):
  - Extended `internal/memory/store.go` to consume model-aware embedding results when available:
    - added result-aware embed helpers (`embedText`, `embedCode`) using `EmbedTextResult` / `EmbedCodeResult`.
    - persisted `embedding_model_id` / `embedding_code_model_id` on legacy rows during create/update/duplicate-update.
  - Added repository dual-write hook for memory rows:
    - writes embedding vectors to per-model table via `EmbeddingRepository.UpsertEmbedding`
    - updates `embedding_index` status to `ready`.
  - Covered with integration test:
    - `internal/memory/memory_integration_test.go::TestMemoryCreate_DualWritesToEmbeddingRepository`
    - validates both `embedding_index` row and model-table row existence after create.
  - Remaining in Step 7:
    - apply the same dual-write integration to knowledge, skills, and entity write paths.
- [x] 2026-04-04: Integrated knowledge write-path dual-write to model tables (Step 7 partial, TDD-first):
  - Extended `internal/knowledge/store.go`:
    - added result-aware embedding support for `embedContent` via `EmbedTextResult`
    - persists active `embedding_model_id` from `EmbedResult.ModelID`
    - dual-writes create/update embeddings into per-model tables using `EmbeddingRepository.UpsertEmbedding`.
  - Added integration coverage:
    - `internal/knowledge/store_dualwrite_integration_test.go::TestKnowledgeCreate_DualWritesToEmbeddingRepository`
    - validates `embedding_index` ready row + model-table row for `knowledge_artifact`.
- [x] 2026-04-04: Integrated skills write-path dual-write to model tables (Step 7 partial, TDD-first):
  - Extended `internal/skills/store.go`:
    - create/update now use `EmbedTextResult`
    - persists `embedding_model_id` from `EmbedResult.ModelID`
    - dual-writes to model tables via `EmbeddingRepository.UpsertEmbedding`.
  - Added integration coverage:
    - `internal/skills/store_dualwrite_integration_test.go::TestSkillsCreate_DualWritesToEmbeddingRepository`
    - validates `embedding_index` ready row + model-table row for `skill`.
- [x] 2026-04-04: Integrated entity upsert dual-write to model tables (Step 7 partial, TDD-first):
  - Extended `db.UpsertEntity` in `internal/db/compat.go`:
    - when `embedding` + `embedding_model_id` are present, it now dual-writes into per-model embedding tables
    - updates `embedding_index` to `ready` via `EmbeddingRepository.UpsertEmbedding`.
  - Added integration coverage:
    - `internal/db/entity_dualwrite_integration_test.go::TestUpsertEntity_DualWritesToEmbeddingRepository`
    - validates `embedding_index` ready row + model-table row for `entity`.
- [x] 2026-04-04: Reduced Step 7 write-path duplication with shared dual-write helper (TDD-first):
  - Added `internal/db/embedding_dualwrite.go` + `internal/db/embedding_dualwrite_test.go`:
    - `db.UpsertEmbeddingIfPresent` centralizes no-op guards and upsert call wiring
    - covered no-op conditions, expected upsert payload, and error propagation.
  - Refactored stores to use shared helper:
    - `internal/memory/store.go` (`dualWriteMemoryEmbeddings`)
    - `internal/knowledge/store.go` (`dualWriteArtifactEmbedding`)
    - `internal/skills/store.go` (`dualWriteSkillEmbedding`).
- [x] 2026-04-04: Started embedding repository layer (Step 6 initial slice, TDD-first):
  - Added `internal/db/embedding_repository.go` with:
    - repository contract types (`EmbeddingQuery`, `ScopeFilter`, `UpsertEmbeddingInput`)
    - `UpsertEmbedding` for per-model table writes + `embedding_index` status update to `ready`
    - `GetEmbedding` for model-table reads
    - `QuerySimilar` ANN query support with scope join/filter wiring.
    - strict object-type and dimension validation
    - safe dynamic table-name validation and model metadata lookup from `embedding_models`.
  - Added integration coverage in `internal/db/embedding_repository_integration_test.go`:
    - successful upsert+read roundtrip
    - dimension mismatch rejection
    - scope-filtered ANN retrieval for memory object type.
  - Remaining in Step 6:
    - complete ANN scope-join coverage across all object types + refactor pass.
- [x] 2026-04-04: Completed Step 6 refactor pass for repository metadata/retry handling (TDD-first):
  - Added `internal/db/retry.go` + `internal/db/retry_test.go`:
    - `runWithRetry` utility with retry classification for transactional conflict errors
    - retries on PostgreSQL `40001` and `40P01`, no retries for non-retryable errors.
  - Added `internal/db/embedding_model_metadata.go`:
    - centralized embedding model metadata lookup helpers
    - shared ready-table resolution + table-name safety checks.
  - Refactored `internal/db/embedding_repository.go`:
    - `UpsertEmbedding` now uses one transaction for model-table upsert + `embedding_index` update
    - write path wrapped with retry helper for serialization/deadlock conflicts
    - switched lookup calls to shared metadata helper.
  - Refactored `internal/db/embedding_bootstrap.go` to use shared model metadata lookup.
  - Added integration coverage:
    - `internal/db/embedding_repository_integration_test.go::TestEmbeddingRepository_UpsertEmbedding_ModelNotReady`.
- [x] 2026-04-04: Completed Step 8 bootstrap resumability/progress refactor (TDD-first):
  - Added integration test:
    - `internal/db/embedding_bootstrap_integration_test.go::TestBootstrapLegacyEmbeddingsForModel_SecondRunSkipsReadyRows`
    - validates resumability by asserting second bootstrap run reports zero additional upserts/index updates.
  - Refactored `internal/db/embedding_bootstrap.go`:
    - each bootstrap source query now skips objects already `ready` in `embedding_index` for the target model
    - added per-stage and final progress logs (`slog.InfoContext`) with upsert/index counters.
- [x] 2026-04-05: Finalized carryover commits for completed embedding update steps:
  - committed previously completed Step 3/4/5 and dual-write integration code that was pending in the worktree
  - includes CLI model management commands/tests, model-driven embedding factory + runtime wiring, registration/integration tests, and entity/memory/knowledge/skills dual-write test coverage.
- [x] 2026-04-05: Started Step 9 dual-read migration with memory recall model-table path (TDD-first):
  - Added integration test:
    - `internal/memory/memory_integration_test.go::TestMemoryRecall_TextSearch_UsesModelTableWhenLegacyEmbeddingMissing`
    - verifies text recall succeeds when legacy inline embedding columns are unusable but model-table embeddings exist.
  - Refactored `internal/memory/recall.go`:
    - text/code/hybrid vector recall now tries active-model repository ANN (`EmbeddingRepository.QuerySimilar`) first with scope filtering
    - falls back to legacy vector SQL recall when model-table results are unavailable (including model not ready/not found) or empty.
- [x] 2026-04-05: Extended Step 9 dual-read migration to knowledge + skills recall (TDD-first):
  - Added integration tests:
    - `internal/knowledge/recall_integration_test.go::TestRecall_UsesModelTableWhenLegacyEmbeddingMissing`
    - `internal/skills/skills_integration_test.go::TestRecall_UsesModelTableWhenLegacyEmbeddingMissing`
  - Refactored vector recall paths:
    - `internal/knowledge/recall.go` now uses repository ANN first for active text model, with legacy vector fallback.
    - `internal/skills/recall.go` now uses repository ANN first for active text model, with legacy vector fallback.
- [x] 2026-04-05: Started Step 10 re-embed pipeline migration to embedding_index pending flow (TDD-first):
  - Added integration coverage in `internal/jobs/reembed_integration_test.go`:
    - `TestReembedJob_RunText_UsesEmbeddingIndexPendingAndMarksReady`
    - `TestReembedJob_RunText_FailureIncrementsRetryAndEventuallyFailed`
  - Refactored `internal/jobs/reembed.go` (`RunText`):
    - now reads pending units from `embedding_index` by active model ID
    - supports object types `memory`, `knowledge_artifact`, `skill`
    - updates legacy embedding columns + model tables
    - marks `embedding_index` rows `ready` on success
    - increments `retry_count`, records `last_error`, and marks `failed` on retry exhaustion.
- [x] 2026-04-05: Completed Step 10 code-path migration to embedding_index pending flow (TDD-first):
  - Added integration coverage in `internal/jobs/reembed_integration_test.go`:
    - `TestReembedJob_RunCode_UsesEmbeddingIndexPendingAndMarksReady`
    - `TestReembedJob_RunCode_FailureIncrementsRetryAndEventuallyFailed`
  - Refactored `internal/jobs/reembed.go` (`RunCode`):
    - now reads pending code-memory units from `embedding_index`
    - updates legacy code embedding + model-table embedding
    - updates status/retry/last_error consistently with text path.
- [x] 2026-04-05: Completed Step 12 documentation/runbook baseline for embedding model operations:
  - Added `docs/embedding-model-operations.md` with:
    - model registration, listing, activation
    - re-embed status monitoring
    - failed-row manual reset SQL
    - rollback procedure and acceptance checks.
  - Linked the runbook from:
    - `docs/README.md`
    - `docs/operations.md`
    - `docs/troubleshooting-playbook.md`.
- [x] 2026-04-04: Added model-aware multi-provider embedder factory primitives and runtime wiring (TDD-first):
  - Added model-driven factory in `internal/embedding/factory.go`:
    - `ModelConfig`, `ModelConfigStore`, `ModelEmbedderFactory`
    - `EmbedderForModel(modelID)` with provider-specific construction for `ollama` and `openai`
    - per-model `service_url` routing and guardrails for OpenAI default URL auth requirements.
  - Added DB-backed model metadata resolver in `internal/embedding/model_store.go`:
    - `GetModelConfig(modelID)` reads provider/runtime fields from `embedding_models`
    - `ActiveModelIDByContentType(text|code)` resolves active models.
  - Extended `EmbeddingService` in `internal/embedding/service.go`:
    - added `EmbedResult{ModelID, Embedding}`
    - added `EmbedTextResult` / `EmbedCodeResult`
    - kept existing `EmbedText` / `EmbedCode` API as compatibility wrappers returning vectors only.
    - added model-factory hook (`SetModelFactory`) plus startup helper `EnableModelDrivenFactory(...)` in `internal/embedding/service_factory_setup.go`.
  - Wired model-driven factory setup during server startup:
    - `cmd/postbrain/main.go` now calls `svc.EnableModelDrivenFactory(ctx, pool, &cfg.Embedding)` after service creation.
  - Added unit coverage:
    - `internal/embedding/factory_test.go`
    - `internal/embedding/model_store_test.go`
    - `internal/embedding/service_test.go` (nil-pool factory setup guard).
- [x] 2026-04-04: Implemented embedding model registration transaction + initial CLI model management commands (TDD-first):
  - Added backend registration flow in `internal/db/embedding_model_registration.go`:
    - `RegisterEmbeddingModel(ctx, pool, params)` validates input and runs one transaction for:
      - model upsert (`ON CONFLICT (slug)` idempotency),
      - optional active-model switch per content type,
      - per-model table provisioning,
      - `table_name`/`is_ready` metadata update,
      - pending-row seeding in `embedding_index` for all existing `memory`/`entity`/`knowledge_artifact`/`skill` objects.
  - Hardened per-model provisioning in `internal/db/embedding_tables.go`:
    - internal transactional helper support,
    - explicit existing-table dimension mismatch detection to fail fast and preserve rollback semantics.
  - Added integration tests in `internal/db/embedding_model_registration_integration_test.go`:
    - success path with full pending-row fanout,
    - slug idempotency,
    - rollback behavior on provisioning dimension mismatch.
  - Added `postbrain-cli embedding-model` command group in `cmd/postbrain-cli/main.go`:
    - `register`, `activate`, `list`
    - supports `--database-url` (or config/env fallback via `--config` + `POSTBRAIN_DATABASE_URL`).
  - Added CLI command tests in `cmd/postbrain-cli/main_test.go` for:
    - register flag validation + success/error behavior,
    - activate flag validation + success behavior.
- [x] 2026-04-04: Added non-breaking embedding schema migration + per-model table provisioning primitives:
  - Added migration `000013_embedding_index`:
    - extends `embedding_models` with provider/runtime metadata (`provider`, `service_url`, `provider_model`, `table_name`, `is_ready`)
    - adds central `embedding_index` table with constraints/index/updated_at trigger
    - intentionally keeps legacy `embedding_model_id` columns for compatibility (cleanup deferred)
  - Added migration integration coverage:
    - `internal/db/embedding_schema_migration_integration_test.go`
  - Added per-model table helpers in `internal/db/embedding_tables.go`:
    - `EmbeddingTableName(modelID)` -> `embeddings_model_<uuid_no_dashes>`
    - `EnsureEmbeddingModelTable(ctx, pool, modelID, dims)` creates table + scope index + HNSW index + updated_at trigger idempotently
  - Added tests:
    - unit: `internal/db/embedding_tables_test.go`
    - integration: `internal/db/embedding_tables_integration_test.go`
- [x] 2026-04-04: Restored styled standalone login layout:
  - Added dedicated `auth_base` template (`internal/ui/web/templates/auth_base.html`) so `/ui/login` remains separate from the sidebar app shell while still loading shared UI styles/scripts.
  - Updated `render()` in `internal/ui/handler.go` to wrap `login` with `auth_base` (instead of raw template output), while preserving HTMX partial behavior.
  - Refined login markup and auth-specific styling:
    - `internal/ui/web/templates/login.html`
    - `internal/ui/web/static/pico.min.css`
  - Added regression test `TestLoginGET_UsesStyledStandaloneLayout` in `internal/ui/handler_auth_test.go`.
- [x] 2026-04-04: Rendered `/ui/login` as a standalone page without app sidebar shell:
  - Added regression test `TestLoginGET_DoesNotRenderAppSidebar` in `internal/ui/handler_auth_test.go`.
  - Updated `internal/ui/handler.go` render logic to bypass base layout for `login` template.
  - Keeps existing HTMX partial behavior unchanged and prevents sidebar/navigation from appearing on unauthenticated login page.
- [x] 2026-04-04: Added detailed embedding re-architecture execution plan:
  - Created `designs/TASKS_EMBEDDING_UPDATE.md` with ordered, TDD-gated action items for:
    - per-model embedding tables (`embeddings_model_<uuid_no_dashes>`)
    - central embedding metadata table + model table mapping
    - multi-provider model registration and re-embedding support
    - fast/aggressive cutover phases and validation gates
  - Captured and locked all previously open design decisions in that task plan.
- [x] 2026-04-04: Unified embedding endpoint config into `embedding.service_url`:
  - Replaced split config keys (`embedding.ollama_url`, `embedding.openai_base_url`) with unified `embedding.service_url`.
  - Updated runtime config struct/defaults and added backward-compat fallback mapping for legacy keys in `internal/config/config.go`.
  - Updated embedding backends to use the unified endpoint:
    - Ollama calls now use `service_url` (fallback `http://localhost:11434`)
    - OpenAI backend uses `service_url` as base URL (fallback default OpenAI API endpoint)
  - Updated service validation:
    - `openai_api_key` required only when `backend=openai` and `service_url` is empty.
  - Updated tests and docs:
    - config/embedding tests and fixtures
    - `config.example.yaml`, `docs/configuration.md`, and `README.md` snippets
- [x] 2026-04-04: Added embedding dimension fitting for knowledge write paths:
  - Added `embedding.FitDimensions(vec, dims)` in `internal/embedding/dimensions.go`:
    - truncates vectors longer than `dims`
    - zero-pads vectors shorter than `dims`
    - keeps vectors unchanged when `dims <= 0` or already equal
  - Added unit coverage in `internal/embedding/dimensions_test.go` (trim/pad/no-op).
  - Updated knowledge embedding code to fit vectors to active text-model dimensions before DB writes:
    - `internal/knowledge/store.go` (`embedContent`, plus chunk embedding path now reuses it)
    - `internal/knowledge/synthesize.go` (`embedContent`)
  - Prevents SQL dimension mismatch failures (for example `expected 1536 dimensions, not 4096`) when using local OpenAI-compatible models that emit larger vectors.
- [x] 2026-04-04: Improved OpenAI-compatible embedding response compatibility:
  - Fixed `internal/embedding/openai.go` decode path to accept both:
    - standard OpenAI shape: `{ "data": [{ "embedding": [...], "index": n }] }`
    - array shape used by some local OpenAI-compatible servers: `[[...], [...]]`
    - indexed object-array shape used by some local servers: `[{ "index": n, "embedding": [[...]] }]`
      (single nested vector is flattened)
    - column-vector nested shape: `[{ "index": n, "embedding": [[x],[y],...] }]`
      (flattened to `[x,y,...]`)
    - matrix nested shape: `[{ "index": n, "embedding": [[...],[...],...] }]`
      (mean-pooled to a single vector)
  - Added single-input compatibility for bare vector/envelope variants.
  - Added regression tests in `internal/embedding/openai_test.go`:
    - `TestOpenAIEmbedder_ArrayResponse_SingleInput`
    - `TestOpenAIEmbedder_ArrayResponse_BatchInput`
    - `TestOpenAIEmbedder_ObjectArrayResponse_NestedEmbedding_SingleInput`
    - `TestOpenAIEmbedder_EnvelopeResponse_NestedEmbedding_SingleInput`
    - `TestOpenAIEmbedder_ObjectArrayResponse_ColumnVector_SingleInput`
    - `TestOpenAIEmbedder_ObjectArrayResponse_MatrixEmbedding_SingleInput`
  - Resolves runtime decode failures like:
    - `json: cannot unmarshal array into Go value of type embedding.openAIResponse`
- [x] 2026-04-04: Added configurable OpenAI-compatible endpoint support for embedding/summarization:
  - Added `embedding.openai_base_url` to runtime config (`internal/config/config.go`) with default empty value.
  - Wired OpenAI constructors in `internal/embedding/service.go` to use `openai_base_url` for text/code/summarize/analyze clients.
  - Added validation for OpenAI backend:
    - requires `embedding.openai_api_key` only when `embedding.openai_base_url` is not configured.
    - allows empty API key for custom OpenAI-compatible endpoints (for example local llama.cpp).
  - Updated OpenAI request handling to omit `Authorization` header when API key is empty.
  - Added test coverage:
    - `internal/config/config_test.go` for `openai_base_url` round-trip + defaults.
    - `internal/embedding/service_test.go` for key requirement + custom base URL acceptance.
    - `internal/embedding/openai_test.go` and `internal/embedding/summarize_test.go` for optional auth-header behavior with custom endpoints.
  - Updated user-facing config docs:
    - `config.example.yaml`
    - `docs/configuration.md`
- [x] 2026-04-04: Added initial AGE PageRank primitive for entity ranking:
  - Added `internal/graph/pagerank.go` with `RunPageRank(ctx, pool)`:
    - validates non-nil DB pool
    - returns `graph.ErrAGEUnavailable` when AGE is not available
    - executes AGE PageRank (`age_pagerank`) over `Entity`/`RELATION`
    - writes scores to `entities.meta["pagerank"]`
  - Added test coverage:
    - unit: `internal/graph/pagerank_test.go` (`nil pool` guard)
    - integration: `internal/graph/pagerank_integration_test.go` (unavailable-mode behavior)
- [x] 2026-04-04: Made MCP AGE graph tooling registration conditional on AGE availability:
  - `internal/api/mcp/server.go` now detects AGE at server startup and only registers `graph_query` when AGE is available.
  - Keeps MCP tool inventory aligned with runtime capabilities instead of exposing unavailable graph tools.
  - Updated integration behavior/tests:
    - `internal/api/mcp/graph_query_integration_test.go` now expects `graph_query` to be absent when AGE is unavailable and present when available.
  - Added regression unit test:
    - `internal/api/mcp/server_test.go` (`TestNewServer_NoPool_DoesNotRegisterGraphQuery`)
- [x] 2026-04-04: Enforced scope authz gates for graph read/query REST endpoints:
  - Added `authorizeRequestedScope` enforcement in:
    - `GET /v1/entities`
    - `GET /v1/graph`
    - `POST /v1/graph/query`
  - Ensures token/principal scope restrictions are applied before graph data access.
  - Extended integration coverage:
    - `internal/api/rest/graph_query_integration_test.go` now includes `TestGraphQuery_DeniesTokenScopeMismatch` expecting HTTP 403 on scope mismatch.
- [x] 2026-04-04: Added MCP `graph_query` tool with AGE-aware execution:
  - Added `internal/api/mcp/graph_query.go` handler:
    - validates `scope` + `cypher`
    - resolves `scope` via `kind:external_id`
    - enforces scope authz via existing MCP scope gates
    - executes scoped Cypher via `graph.RunCypherQuery`
    - maps unavailable AGE to tool error (`graph_query: AGE unavailable`)
  - Registered tool in `internal/api/mcp/server.go`:
    - name: `graph_query`
    - args: `scope`, `cypher`
  - Added tests:
    - unit: `internal/api/mcp/graph_query_test.go`
    - integration: `internal/api/mcp/graph_query_integration_test.go`
- [x] 2026-04-04: Implemented REST Cypher query endpoint using AGE runtime support:
  - Replaced `/v1/graph/query` 501 stub in `internal/api/rest/graph.go` with full handler logic:
    - request JSON parsing (`cypher`, `scope_id`)
    - input validation (`invalid request body`, `cypher is required`, `invalid scope_id`)
    - AGE execution via `graph.RunCypherQuery(...)`
    - `ErrAGEUnavailable` mapped to `501 AGE unavailable`
    - successful response shape: `{ "rows": [...] }`
  - Added tests:
    - unit: `internal/api/rest/graph_test.go` (`queryCypher` validation and nil-pool behavior)
    - integration: `internal/api/rest/graph_query_integration_test.go` (AGE-aware 501/200 behavior end-to-end)
- [x] 2026-04-04: Implemented AGE overlay sync primitives in `internal/graph/age_sync.go`:
  - Added `SyncEntityToAGE(ctx, pool, entity)`:
    - `MERGE` vertex by `id`
    - upserts scope/entity metadata (`scope_id`, `entity_type`, `name`, `canonical`)
    - returns `ErrAGEUnavailable` when AGE is not installed
  - Added `SyncRelationToAGE(ctx, pool, rel)`:
    - `MATCH` endpoints by `subject_id`/`object_id`
    - `MERGE` edge by `(subject,predicate,object)` using `RELATION`
    - upserts edge metadata (`confidence`, `scope_id`)
    - returns `ErrAGEUnavailable` when AGE is not installed
  - Added integration coverage:
    - `internal/graph/age_sync_integration_test.go` validates unavailable-mode behavior and successful entity+relation sync when AGE is present.
- [x] 2026-04-04: Implemented initial AGE query support primitives in `internal/graph`:
  - Added `DetectAGE(ctx, pool) bool` for runtime AGE availability detection.
  - Added `ErrAGEUnavailable` and `RunCypherQuery(ctx, pool, scopeID, cypher)`:
    - executes Cypher via AGE `cypher('postbrain', ...)`
    - applies a scope anchor prefix for scoped traversal entry (`MATCH (n:Entity {scope_id: ...}) WITH n ...`)
    - normalizes result rows into `[]map[string]any`
  - Added tests:
    - unit: `internal/graph/age_query_test.go` (scope prefix builder)
    - integration: `internal/graph/age_query_integration_test.go` (AGE available/unavailable behavior and scoped row filtering)
- [x] 2026-04-04: Added runtime AGE overlay bootstrap so AGE can be enabled after initial migration:
  - Added `db.EnsureAGEOverlay(ctx, pool)` in `internal/db/age_overlay.go`:
    - idempotent best-effort `CREATE EXTENSION age` + `LOAD 'age'` + `create_graph('postbrain')`
    - safely degrades with NOTICE when AGE is unavailable
  - Wired AGE bootstrap into migration flow:
    - `internal/db/migrate.go` now runs `EnsureAGEOverlay` after `migrate.Up()` (including no-change runs)
  - Wired server startup fallback when `database.auto_migrate=false`:
    - `cmd/postbrain/main.go` now calls `EnsureAGEOverlay` during startup in that mode
  - Added integration regression test:
    - `internal/db/age_overlay_integration_test.go` validates no-error/idempotent behavior and asserts graph presence when AGE exists.
- [x] 2026-04-04: Unified recall behavior across MCP and UI query playground, with safer graph defaults:
  - Added shared retrieval orchestrator:
    - `internal/retrieval/orchestrate.go`
    - centralizes memory/knowledge/skill recall + merge + optional graph-context augmentation
  - Switched MCP `recall` to use the shared orchestrator instead of duplicating per-layer retrieval logic.
  - Set `graph_depth` default to `1` (still user-overridable and capped at `2`) and added tests for parse/default/cap behavior:
    - `internal/api/mcp/recall_test.go`
  - Updated MCP tool metadata for `graph_depth` to reflect the new default.
  - Updated UI query handler to use the same shared orchestrator so behavior matches MCP recall semantics.
  - Added integration regression coverage to ensure selected-scope query includes ancestor-scope memories:
    - `internal/ui/handler_query_integration_test.go`
  - Improved query playground toolbar layout/styling for clearer responsive behavior:
    - `internal/ui/web/templates/query.html`
    - `internal/ui/web/static/pico.min.css`
  - Improved code-mode memory recall fallback to include FTS results when code embeddings are unavailable/empty, with unit tests:
    - `internal/memory/recall.go`
    - `internal/memory/recall_test.go`
- [x] 2026-04-04: Extended local development PostgreSQL image to include Apache AGE:
  - Added `deploy/docker/postgres-dev/Dockerfile` based on `pgvector/pgvector:pg18` that builds and installs Apache AGE from source.
  - Switched `docker-compose.yml` `postgres` service from a direct image reference to a local build using the new Dockerfile.
  - Enabled AGE preload in Postgres startup flags:
    - `shared_preload_libraries=age,pg_cron,pg_partman_bgw`
  - Updated init bootstrap SQL (`scripts/postgres-init.sql`) to create the AGE extension alongside existing `pg_cron`, `pg_partman`, and `vector`.
  - Updated `README.md` prerequisites/quickstart wording to reflect that the compose dev DB includes AGE.
- [x] 2026-04-04: Fixed promotions queue visibility and filtering in Web UI (TDD-first):
  - Updated `/ui/promotions` handler to support combined filters:
    - `scope_id` (specific scope or all scopes)
    - `status` (`all`, `pending`, `approved`, `rejected`, `merged`)
  - Changed default promotions view to `status=all` so non-pending requests are visible by default.
  - Added strict query validation and explicit error handling:
    - invalid `scope_id` -> `400 invalid scope id`
    - invalid `status` -> `400 invalid status`
    - DB load failures -> `500` (instead of silently rendering empty results)
  - Extended promotions template with:
    - scope selector
    - status selector
    - target scope column in result rows
  - Added tests:
    - unit: `internal/ui/handler_promotions_test.go`
    - integration: `internal/ui/handler_promotions_integration_test.go` (scope filter + approved visibility regression)
- [x] 2026-04-04: Fixed scope deletion failure caused by promotion request FK constraint (TDD-first):
  - Added DB regression test `internal/db/scope_delete_promotion_integration_test.go` reproducing:
    - deleting a scope referenced by approved promotion requests fails with FK violation
  - Added migration:
    - `000012_promotion_scope_fk_cascade.up.sql`
    - `000012_promotion_scope_fk_cascade.down.sql`
  - Changed `promotion_requests.target_scope_id` FK to `ON DELETE CASCADE`, allowing scope deletion without manual promotion cleanup.
- [x] 2026-04-04: Extended project landing page content and flow design:
  - Added public installer script for docs hosting:
    - `site/public/install-postbrain.sh`
  - Updated homepage quickstart to use hosted one-liner install commands from `https://simplyblock.github.io/postbrain/install-postbrain.sh`.
  - Repositioned quickstart below hero to preserve the enterprise-oriented value proposition prominence.
  - Refined quickstart checklist presentation:
    - two-column checkmark list on desktop
    - added team collaboration step (“Collaborate with team members in shared scopes”)
  - Expanded homepage information architecture in `site/src/pages/index.astro`:
    - explicit supported agent integrations
    - explicit model/backend support framing
    - added missing “Skill Sharing” and “MCP + REST Interfaces” value points
  - Reworked “Typical Flow” from a plain list into a horizontal timeline with separate explanatory cards underneath.
- [x] 2026-04-04: Redesigned docs site root page (`/`) into a project landing page:
  - Replaced the simple docs-index card list in `site/src/pages/index.astro` with a presentable product-style homepage.
  - Added:
    - top navigation (`Getting Started`, `Documentation`, `GitHub`)
    - hero with value proposition and direct call-to-action buttons
    - quickstart block with concrete bootstrap commands
    - capability overview cards (`memory`, `knowledge`, retrieval, scope governance, tooling, deployment)
    - typical workflow section and curated documentation entry points
  - Kept visual direction aligned with existing docs/web style (dark palette, accent color, card-based surfaces).
- [x] 2026-04-04: Polished GitHub Pages docs UX and content framing:
  - Reworked `site/src/layouts/DocsLayout.astro` to improve readability and navigation:
    - sidebar grouping by sections parsed from `docs/README.md`
    - removed duplicate docs index entry from sidebar link list
    - increased visual separation between sidebar sections
    - left-aligned main docs content column (instead of centered)
    - improved code block rendering (balanced padding, compact but readable line metrics, fixed Shiki newline spacing artifacts)
    - improved table styling for reference pages (header contrast, row striping, spacing, hover state, mobile overflow)
  - Refined `docs/introduction.md` narrative and explicitly documented that default agent memory is typically bound to a specific user/account/device/agent context and not naturally shared across teams/org scopes.
- [x] 2026-04-04: Expanded first-run documentation with complete bootstrap flow:
  - Updated `docs/getting-started.md` with a practical initial configuration runbook covering:
    - bootstrap via `postbrain onboard` (admin principal + first token)
    - initial principal hierarchy creation (`company`, `team`, `user`)
    - membership chain setup (`user -> team -> company`)
    - initial scope hierarchy creation (`company`, `team`, `project`, `user`)
    - project token creation, repository attachment, first sync trigger, and sync status checks
    - initial Codex/Claude skill install + skill sync guidance
  - Updated `docs/api-auth-examples.md` token creation command to include required `--principal` argument.
- [x] 2026-04-03: Fixed GitHub Pages docs link routing for Astro-generated pages:
  - Updated `site/scripts/sync-docs.mjs` to rewrite all relative markdown links from `*.md` to route-style `*/` paths during docs sync.
  - Covers plain and relative markdown links (e.g. `foo.md`, `./foo.md`, `../foo.md#section`) while leaving absolute URLs and in-page anchors untouched.
  - Prevents broken navigation on `https://simplyblock.github.io/postbrain/` caused by unresolved `.md` links.
- [x] 2026-04-03: Added initial Helm deployment chart with generated runtime config:
  - Added chart scaffold under `deploy/helm/postbrain` (`Chart.yaml`, `values.yaml`, templates).
  - Added generated Postbrain `config.yaml` from Helm values via template helper and mounted as Kubernetes Secret.
  - Added Deployment, Service, ServiceAccount, optional Ingress templates.
  - Set default image repository to Docker Hub `simplyblock/postbrain`.
- [x] 2026-04-03: Added Gateway API HTTPRoute support to Helm routing and enforced valid routing mode:
  - Added optional `gateway.networking.k8s.io/v1` `HTTPRoute` template.
  - Added routing validation to fail render when neither or both of Ingress/HTTPRoute are enabled.
  - Added `httpRoute.*` values (`enabled`, `parentRefs`, `hostnames`, `pathPrefix`) and defaulted chart to Ingress enabled.
- [x] 2026-04-03: Made `build-package` workflow depend on successful `CI` workflow completion:
  - Switched `.github/workflows/build-package.yml` trigger to `workflow_run` on `CI` (`completed`) plus manual dispatch.
  - Guarded jobs to run only when upstream CI conclusion is `success`.
  - Checked out and built the exact `workflow_run.head_sha` commit to guarantee artifact provenance.
  - Kept tag release behavior by resolving tags on the CI-tested commit and releasing only when a tag exists.
  - Updated `.github/workflows/ci.yml` to run on `v*` tags so release-tag commits still execute CI first.
- [x] 2026-04-03: Switched Docker publishing to artifact-based image builds:
  - Added `Dockerfile.release` that consumes prebuilt `postbrain` binaries from `dist/linux-<arch>/postbrain` instead of recompiling in-image.
  - Updated `docker-publish` job in `.github/workflows/build-package.yml` to:
    - download the merged `release-artifacts` artifact
    - extract `postbrain-server_linux_{amd64,arm64}.tar.gz` into `dist/linux-{amd64,arm64}`
    - build and push a multi-arch image with `docker/build-push-action` using `Dockerfile.release`
  - Preserved Docker Hub tagging strategy (`sha-*`, semantic/tag aliases, `latest` on `main` push flows).
- [x] 2026-04-03: Fixed Helm container command wiring for server startup:
  - Updated `deploy/helm/postbrain/templates/deployment.yaml` to set explicit `command: ["postbrain"]`.
  - Keeps existing args (`serve --config /etc/postbrain/config.yaml`) so the runtime entrypoint no longer attempts to execute a non-existent `serve` binary.
  - Resolves pod startup failure: `exec serve failed: No such file or directory`.
- [x] 2026-04-03: Added optional cert-manager `Certificate` resource to Helm chart:
  - Added `deploy/helm/postbrain/templates/certificate.yaml` gated by `certificate.enabled`.
  - Added certificate values in `deploy/helm/postbrain/values.yaml`:
    - `certificate.name`, `labels`, `secretName`
    - `certificate.privateKey.rotationPolicy`
    - `certificate.issuerRef.{name,kind,group}`
    - `certificate.dnsNames`
  - Enables values-driven certificate provisioning for ingress/gateway TLS secret management.
- [x] 2026-04-03: Added optional Gateway API `Gateway` resource to Helm chart for HTTPRoute deployments:
  - Added `deploy/helm/postbrain/templates/gateway.yaml` gated by `gateway.enabled`.
  - Added `gateway.*` values for name, class, labels/annotations, and listener configuration (protocol, port, hostname, allowedRoutes, tls/certificateRefs).
  - Updated `HTTPRoute` template to auto-reference the chart-managed gateway when `httpRoute.parentRefs` is unset and `gateway.enabled=true`.
  - Tightened validation:
    - when `httpRoute.enabled=true`, require either `httpRoute.parentRefs` or `gateway.enabled=true`
    - when `gateway.enabled=true`, require non-empty `gateway.gatewayClassName`.
- [x] 2026-04-03: Extended public Helm deployment docs in server installation:
  - Expanded `docs/server-installation.md` Helm section with:
    - install flow and prerequisites
    - routing modes (Ingress, HTTPRoute+existing Gateway, HTTPRoute+chart-managed Gateway)
    - certificate integration with cert-manager and secret reference guidance
    - operational troubleshooting for common Helm/Gateway/certificate issues
  - Kept docs navigation focused on `Server Installation` as the primary deployment entrypoint.
- [x] 2026-04-03: Rebalanced installation docs to keep server/client responsibilities clear:
  - Kept `docs/server-installation.md` focused on server runtime deployment paths only.
  - Moved explicit `postbrain-cli` installation guidance into `docs/getting-started.md`:
    - release client archives
    - Linux client packages (`.deb`, `.rpm`)
    - package-manager manifest locations (Homebrew, MacPorts, winget)
  - Updated step ordering in getting-started to install client tooling before skill installation.
- [x] 2026-04-03: Added release installer helper script for server/client binaries:
  - Added `scripts/install-postbrain.sh` with automatic OS/arch detection:
    - OS: `linux`, `darwin`
    - arch: `amd64`, `arm64`
  - Supports component selection:
    - `server` -> installs `postbrain`
    - `client` -> installs `postbrain-cli`
  - Supports version pinning via arg/env (`vX.Y.Z`) and defaults to latest GitHub release tag.
  - Updated install docs to reference script usage as an alternative to manual downloads/package managers.
- [x] 2026-04-03: Expanded public documentation coverage for production operations and lifecycle:
  - Added pages:
    - `docs/upgrade-guide.md`
    - `docs/compatibility.md`
    - `docs/release-policy.md`
    - `docs/backup-restore.md`
    - `docs/production-hardening.md`
    - `docs/monitoring-alerting.md`
    - `docs/performance-tuning.md`
    - `docs/platform-quickstarts.md`
    - `docs/api-auth-examples.md`
    - `docs/troubleshooting-playbook.md`
    - `docs/uninstall-cleanup.md`
  - Updated docs index (`docs/README.md`) to include the new operational/public guidance pages.
  - Intentionally excluded HA/scaling guidance per current documentation scope decision.
- [x] 2026-04-03: Added dedicated indexing model documentation:
  - Added `docs/indexing-model.md` explaining:
    - indexed item types (memories, artifacts, chunks, entities, relations)
    - artifact chunk graph relations (`chunk_of`, `next_chunk`)
    - scope-aware indexing boundaries
    - hybrid retrieval usage of indexed data (vector/FTS/trigram/graph context)
    - lifecycle and maintenance behavior
  - Linked indexing docs from:
    - `docs/README.md`
    - `docs/architecture-overview.md`
    - `docs/configuration.md`
- [x] 2026-04-03: Enriched public docs from list-style notes to explanatory guidance:
  - Expanded newly added operations/lifecycle pages with practical narrative text (not only bullet inventories), including rationale, decision guidance, and validation notes.
  - Added deeper explanatory flow to `docs/indexing-model.md`, including an end-to-end indexing/retrieval example.
  - Improved readability and actionability across:
    - upgrade, compatibility, release policy
    - backup/restore, hardening, monitoring, performance
    - quickstarts, API auth, troubleshooting, uninstall
- [x] 2026-04-03: Added minimal Astro docs site and GitHub Pages publication pipeline:
  - Added `site/` Astro project with:
    - `site/package.json`
    - `site/astro.config.mjs`
    - `site/src/pages/index.astro`
    - `site/scripts/sync-docs.mjs`
  - Site build uses repository `docs/` directory as source content by syncing markdown files into Astro pages at build time.
  - Extended `.github/workflows/build-package.yml` with:
    - `docs-pages-build` job (Node setup + Astro build + pages artifact upload)
    - `docs-pages-deploy` job (`actions/deploy-pages`) for GitHub Pages publication
    - deployment gating to successful CI `workflow_run` on main pushes (and manual dispatch support)
  - Updated `.gitignore` for generated site content and node dependencies.
- [x] 2026-04-03: Reorganized public docs with dedicated server installation guide:
  - Added `docs/server-installation.md` with explicit install paths for:
    - local process/source build
    - GitHub release binary downloads
    - Docker image (`simplyblock/postbrain`)
    - Kubernetes Helm chart
  - Updated `docs/getting-started.md` to reference the new installation guide instead of embedding install instructions inline.
  - Updated `docs/README.md` start-here navigation to include `Server Installation`.
- [x] 2026-04-03: Fixed social-login integration test config regression:
  - Updated `internal/ui/oauth_social_integration_test.go` to set `oauth.social.auto_create_users=true` in the default social-login E2E fixture.
  - Aligns test behavior with runtime defaults from config loader and restores expected `200` success path for first-time social login.
- [x] 2026-04-03: Added UI support to edit principals (including users) with slug/display-name updates:
  - Added `POST /ui/principals/{id}` handler path and validation/error handling in `internal/ui/handler.go`.
  - Added principals page edit UI with per-row `Edit` action and edit dialog in `internal/ui/web/templates/principals.html`.
  - Added `principals.Store.UpdateProfile(...)` to update `slug` + `display_name` in one operation.
  - Added tests:
    - unit: `internal/ui/handler_principals_test.go` (invalid id, validation, nil-pool, dialog render)
    - integration: `internal/ui/handler_principals_integration_test.go` (successful update + redirect + persisted changes)
- [x] 2026-04-03: Added container image build for Postbrain server with `markitdown` + `gopls`:
  - Added multi-stage root `Dockerfile` that builds `postbrain`, installs pinned `gopls` (`v0.21.1`), and installs pinned `markitdown[all]` (`0.1.5`).
  - Hardened runtime image to run as a dedicated non-root user (`UID/GID 10001`) with owned config/state directories.
  - Added `.dockerignore` to reduce build context size and exclude local artifacts.
  - Updated `docker-compose.yml` `postbrain` service build args for `GOPLS_VERSION` and `MARKITDOWN_VERSION`.
  - Removed obsolete compose env `POSTBRAIN_SERVER_TOKEN` and retained DB bootstrap env wiring.
  - Added `docker-build` make target to produce `postbrain:latest` with pinned tool versions.
  - Added GitHub Actions CI job `docker-build` (in `.github/workflows/ci.yml`) to build the Docker image on push/PR.
- [x] 2026-04-03: Added cross-platform binary build matrix and initial package scaffolding:
  - Added `make build-target`, `make build-cross`, and `make build-archives` for `postbrain` and `postbrain-cli` across:
    - `linux` (`amd64`, `arm64`)
    - `darwin` (`amd64`, `arm64`)
    - `windows` (`amd64`, `arm64`)
  - Updated CI build job to run `make build-archives` and upload `dist/**` artifacts.
  - Added no-CGO extractor fallback in `internal/codegraph` with build tags:
    - tree-sitter extractors are now `//go:build cgo`
    - `extract_nocgo.go` provides graceful unsupported-language fallbacks for non-Go languages when `CGO_ENABLED=0`
    - added no-CGO regression test `internal/codegraph/extract_nocgo_test.go`
  - Split package scaffolding into `postbrain-server` and `postbrain-client`:
    - build archives now emit per-component artifacts:
      - `postbrain-server_<os>_<arch>.(tar.gz|zip)`
      - `postbrain-client_<os>_<arch>.(tar.gz|zip)`
    - Linux package inputs:
      - `packaging/nfpm/postbrain-server.yaml`
      - `packaging/nfpm/postbrain-client.yaml`
      - `packaging/debian/control` with two binary package stanzas
      - `packaging/redhat/postbrain-server.spec` and `postbrain-client.spec`
    - macOS package inputs:
      - `packaging/homebrew/postbrain-server.rb` and `postbrain-client.rb`
      - `packaging/macports/postbrain-server/Portfile` and `postbrain-client/Portfile`
    - Windows package inputs:
      - `packaging/winget/Simplyblock.PostbrainServer*.yaml`
      - `packaging/winget/Simplyblock.PostbrainClient*.yaml`
- [x] 2026-04-03: Reworked CI build/release pipeline split:
  - Restored `.github/workflows/ci.yml` build job to simple `make build` to validate full application build in one step.
  - Added dedicated `.github/workflows/build-package.yml` pipeline for release engineering:
    - multi-OS/multi-arch matrix server build (`linux`, `darwin`, `windows` × `amd64`, `arm64`) with `CGO_ENABLED=1`
    - per-target archive generation via `make package-target`
    - merge all matrix outputs into a unified artifact set
    - Linux package generation (`.deb` + `.rpm`) for `postbrain-server` and `postbrain-client` via `nfpm`
    - checksum generation and GitHub release publication with all merged artifacts
  - Added reusable `make package-target GOOS=<os> GOARCH=<arch>` target used by both local and CI packaging flows.
  - Fixed `build-package` merge + packaging reliability:
    - corrected merged Linux binary path to `collected/dist/linux-*` after artifact download
    - render `packaging/nfpm/*.yaml` per-arch/per-version before invoking `nfpm` so `${ARCH}` / `${VERSION}` placeholders resolve consistently
    - made merge step path-agnostic by discovering Linux archives recursively in downloaded artifacts and reconstructing `dist/linux-{amd64,arm64}` from tarballs before `nfpm` runs
- [x] 2026-04-03: Added build-version injection via linker flags:
  - `cmd/postbrain` and `cmd/postbrain-cli` now expose explicit `version` commands that print build metadata:
    - semantic build version
    - git short ref
    - build timestamp (UTC)
  - Added test coverage:
    - `cmd/postbrain/main_test.go` (`postbrain version`)
    - `cmd/postbrain-cli/main_test.go` (`postbrain-cli version`)
  - `Makefile` now injects `buildVersion`, `buildGitRef`, and `buildTimestamp` using `-ldflags -X ...` for all build targets (`build`, `build-target`), defaulting to:
    - `VERSION = git describe --tags --always --dirty`
    - `GIT_REF = git rev-parse --short HEAD`
    - `BUILD_TIMESTAMP = date -u +%Y-%m-%dT%H:%M:%SZ`
  - `build-package` matrix builds now pass the resolved CI release/build version to `make build-target VERSION=...` so binaries and package versions stay aligned.
- [x] 2026-04-03: Added configurable social user provisioning policy:
  - Added `oauth.social` config block:
    - `auto_create_users` (default `true`)
    - `require_verified_email` (default `false`)
    - `allowed_email_domains` (default empty allow-all)
  - Social callback now enforces optional verified-email and domain allowlist checks before principal linking.
  - Added social identity policy support:
    - `FindOrCreateWithPolicy(..., AutoCreateUsers=true|false)`
    - when disabled, social login links only pre-provisioned principals by email slug and returns `account is not provisioned` otherwise
  - Extended social user metadata with provider-level fields (`EmailVerified`, `HostedDomain`) and populated Google claims (`email_verified`, `hd`).
  - Added tests:
    - unit: domain allowlist helper
    - integration: auto-create disabled requires pre-provisioned principal
    - integration: identity-store pre-provisioned link + unprovisioned rejection
- [x] 2026-04-03: Aligned runtime config examples/docs with current config code:
  - Updated `config.example.yaml` to match `internal/config/config.go` keys:
    - added missing supported keys: `embedding.summary_model`, `jobs.chunk_backfill_enabled`
    - removed unsupported legacy key: `server.token`
  - Added concise per-property comments in `config.example.yaml` for public documentation quality.
  - Updated stale config loader comment in `internal/config/config.go` to reflect current validation (`database.url` required).
  - Normalized trailing newline formatting in related `internal/postbraincli` files.
- [x] 2026-04-03: Added public user-oriented documentation set under `docs/`:
  - Added multi-file docs structure:
    - `docs/README.md` (index)
    - `docs/introduction.md`
    - `docs/architecture-overview.md`
    - `docs/getting-started.md`
    - `docs/configuration.md`
    - `docs/oauth-logins.md`
    - `docs/using-with-coding-agents.md`
    - `docs/using-with-chatgpt.md`
    - `docs/common-workflows.md`
    - `docs/security.md`
    - `docs/operations.md`
    - `docs/faq.md`
  - Updated content to public/user-facing language (less internal implementation framing).
  - Documented common real-world workflows and linked them from onboarding guides.
  - Added dedicated OAuth login + OAuth server configuration/validation documentation.
  - Updated configuration docs to reflect current schema semantics (`server.token` removed).
- [x] 2026-04-03: Updated README with Getting Started instructions for skill installation:
  - Added a dedicated `Getting Started` section under Claude connection docs.
  - Documented both installer commands:
    - `postbrain-cli install-codex-skill --target ...`
    - `postbrain-cli install-claude-skill --target ...`
  - Clarified that both can be installed in mixed-agent repositories.
- [x] 2026-04-03: Renamed CLI command directory from `cmd/postbrain-hook` to `cmd/postbrain-cli` to match binary/command naming.
  - Updated build wiring in `Makefile` to build both `postbrain-hook` and `postbrain-cli` from `./cmd/postbrain-cli`.
  - Updated repository structure reference in `README.md`.
  - Updated task-list path references to the new directory.
- [x] 2026-04-03: Generalized hook CLI toward `postbrain-cli` and removed checkout dependency for Codex skill install:
  - `cmd/postbrain-cli` root command now uses `postbrain-cli` with alias `postbrain-hook` for compatibility.
  - Added `install-codex-skill` command that installs `.codex/skills/postbrain.md` from embedded CLI assets.
  - Added tested installer helper package `internal/postbraincli` with unit tests for install + AGENTS marker behavior.
  - Updated `scripts/install-codex-skill.sh` to delegate to `postbrain-cli` when available (fallback to file-copy mode).
  - Updated `Makefile build` to emit `postbrain-cli` alongside `postbrain` and `postbrain-hook`.
  - Updated README hook/sync examples to prefer `postbrain-cli` and documented `install-codex-skill`.
- [x] 2026-04-03: Updated `.codex/skills/postbrain.md` with stricter MCP memory workflow policy:
  - Startup scope bootstrap now checks `.codex/postbrain-base.md`, `README.md`, `AGENTS.md`, and common docs before prompting.
  - Added explicit opt-in/opt-out persistence to `.codex/postbrain-base.md` when no scope is defined.
  - Made `recall` mandatory before each new task and `remember` mandatory after each tool step/sub-task.
  - Added guidance to use `summary` + detailed `content` in memories with precise, extensive entity tagging.
  - Clarified memory vs artifact usage (iteration vs long-lived decisions/designs).
  - Added Postbrain registry skill discovery/install expectations via `skill_search`/`skill_install`.
- [x] 2026-04-03: Added artifact chunk entities and linear chunk graph relations (TDD-first):
  - Added integration test `internal/knowledge/store_chunk_graph_integration_test.go` asserting chunk graph shape on artifact create:
    - `artifact` entity exists with canonical `artifact:<artifact_id>`
    - `artifact_chunk` entity exists per chunk with canonical `artifact:<artifact_id>:chunk:<n>`
    - each chunk has exactly one `chunk_of` relation to the artifact entity
    - chunks are connected by `next_chunk` only (linear chain), with no `chunk_sibling` relations
  - Extended `internal/knowledge/store.go:createChunks` to best-effort upsert and connect:
    - artifact entity (`entity_type=artifact`)
    - chunk entities (`entity_type=artifact_chunk`)
    - `chunk_of` (`chunk -> artifact`) and `next_chunk` (`chunk_n -> chunk_n+1`) relations
    - relation/entity upsert/link failures are logged and do not block artifact writes
- [x] 2026-04-02: Post-implementation OAuth verification fixes (design parity):
  - `internal/ui/auth.go` + login template now preserve and honor `next` so `/ui/login?next=...` resumes consent flow after token login.
  - `internal/oauth/server.go` token exchange updated so confidential clients can rely on valid `client_secret` without mandatory PKCE verifier (PKCE remains enforced for public clients).
  - `internal/ui/oauth_consent.go` + `oauth_consent.html` now render human-readable scope labels on consent screen.
  - Added/updated tests:
    - `internal/ui/handler_auth_test.go` (hidden `next` field render)
    - `internal/ui/auth_integration_test.go` (login redirect to `next`)
    - `internal/oauth/server_test.go` (confidential client valid secret + no verifier succeeds)
    - `internal/ui/oauth_consent_test.go` (human-readable scope labels assertion)
- [x] 2026-04-02: Completed OAuth implementation baseline and integrations (`designs/TASKS_OAUTH.md` all items checked):
  - Implemented OAuth/social data model, config, sqlc queries, and core authorization-server package (`internal/oauth`).
  - Implemented social provider stack and identity linking (`internal/social`) with integration tests.
  - Implemented UI OAuth social-login and consent flows, templates, route wiring, and OAuth-aware UI handler construction.
  - Wired OAuth server + social dependencies in `cmd/postbrain/main.go` with route registration (`/.well-known/oauth-authorization-server`, `/oauth/*`).
  - Added integration coverage for:
    - full Authorization Code + PKCE flow (including replay, PKCE mismatch, redirect mismatch, confidential-client bad secret, registration disabled)
    - social login E2E via mock provider with real DB and cookie-authenticated `/ui` access.
  - Audited code paths to ensure `social_identities.raw_profile` is stored but not exposed through API/UI responses or logs.
- [x] 2026-04-02: Started OAuth implementation from `designs/DESIGN_OAUTH.md` (TDD-first), completed Phases 1–3 baseline:
  - Phase 1 (DB + config):
    - Added migrations `000011_oauth.up/down.sql` with `social_identities`, `oauth_clients`, `oauth_auth_codes`, `oauth_states` (+ indexes/triggers/checks).
    - Extended migration integration tests to verify OAuth table presence, key constraints, and explicit `000011` down/up roundtrip behavior.
    - Added OAuth config structs/defaults (`oauth.server.auth_code_ttl`, `state_ttl`, `token_ttl`, `dynamic_registration`) and config tests.
    - Extended `config.example.yaml` with full `oauth:` provider and server blocks.
  - Phase 2 (sqlc query layer):
    - Added query files: `oauth_clients.sql`, `oauth_codes.sql`, `oauth_states.sql`, `social_identities.sql`.
    - Regenerated sqlc outputs and added integration tests for consume-once, expiry rejection, and revoked-client lookup exclusion.
  - Phase 3 (`internal/oauth` core package):
    - Implemented `pkce.go`, `scopes.go`, `states.go`, `clients.go`, `codes.go`, `token_exchange.go`, `metadata.go`, `server.go`.
    - Added comprehensive unit tests for PKCE, scope parsing, state lifecycle/hash behavior, client registration/lookup/revoke, code lifecycle/PKCE verification, token issuance, metadata, and HTTP handlers (`/oauth/*`, `/.well-known/oauth-authorization-server`).
    - Updated `designs/TASKS_OAUTH.md` to mark completed tasks and reconcile validation/security expectations against design.
- [x] 2026-04-02: Started scope-security hardening Finding 1 with TDD:
  - Added shared API helper package `internal/api/scopeauth`
  - Added `AuthorizeRequestedScope(token, requestedScopeID, effectiveScopeIDs)` enforcing both:
    - token `scope_ids` restrictions (`auth.EnforceScopeAccess`)
    - principal effective-scope inclusion
  - Added unit tests covering allow/deny matrix and explicit sentinel error assertions
- [x] 2026-04-02: Completed Finding 1 scope-auth wiring for scope-string handlers (TDD-first):
  - Added context-aware auth helper wiring in `internal/api/rest/scopeauth.go` and `internal/api/mcp/scopeauth.go`
  - Enforced scope authorization in all identified scope-taking REST/MCP handlers before reads/writes
  - Added inventory guard tests ensuring scope-taking handlers call `authorizeRequestedScope`:
    - `internal/api/rest/scopeauth_inventory_test.go`
    - `internal/api/mcp/scopeauth_inventory_test.go`
- [x] 2026-04-02: Fixed MCP integration fixtures after scope-auth hardening:
  - `internal/api/mcp/mcp_integration_test.go` now injects authenticated token context (`auth.ContextKeyToken`) alongside principal ID
  - Test token includes explicit `scope_ids` for the created scope so scope auth reflects real authenticated requests
  - Verified `make test-integration` passes end-to-end
- [x] 2026-04-02: Added GitHub Actions CI workflow:
  - `.github/workflows/ci.yml` with dedicated jobs for:
    - quick unit tests (`make test`)
    - integration tests (`make test-integration`)
    - build (`make build`)
    - format check (`make fmt` + `git diff --exit-code`)
    - vet (`make vet`)
- [x] 2026-04-02: Started Go LSP (`gopls`) TCP resolver implementation for codegraph:
  - Added optional LSP stage in `internal/codegraph/Resolver` between import-aware and suffix fallback
  - Added `internal/codegraph/GoplsTCPResolver` with JSON-RPC/LSP over TCP:
    - handshake: `initialize` / `initialized`
    - lookup: `workspace/symbol`
    - teardown: `shutdown` / `exit`
  - Added indexer opt-in wiring:
    - `IndexOptions.GoLSPAddr`
    - `IndexOptions.GoLSPTimeout`
    - per-run resolver lifecycle with graceful fallback when LSP is unavailable
  - Added unit tests:
    - `internal/codegraph/resolve_lsp_test.go`
    - `internal/codegraph/gopls_tcp_test.go`
    - `internal/codegraph/indexer_lsp_test.go`
- [x] 2026-04-02: Added Go LSP integration proof test (real `gopls`, integration tag):
  - `internal/codegraph/lsp_integration_test.go`
  - Starts `gopls serve` on a local TCP port (skips if binary unavailable)
  - Indexes the same Go fixture with LSP disabled/enabled
  - Seeds competing suffix candidates and asserts enabled run resolves `calls` target via LSP path
  - Added `GoLSPRootURI` wiring and `textDocument/definition`-first resolver path for file-context resolution
- [x] 2026-04-02: Added parallel 3D entity graph UI view:
  - New authenticated route `GET /ui/graph3d` handled by `handleGraph3D`
  - Shared graph data loader reused by both 2D (`/ui/graph`) and 3D views
  - New template `internal/ui/web/templates/graph3d.html` using `3d-force-graph`
  - Navigation updated in base template with `Entity Graph 3D` link
  - Unit tests added in `internal/ui/handler_graph_test.go` (direct render + unauth redirect)
- [x] 2026-04-02: Locked 3D graph camera panning in `/ui/graph3d`:
  - `controls.enablePan = false`
  - keep rotation + zoom enabled (`enableRotate`, `enableZoom`)
- [x] 2026-04-02: Added automatic initial/final 3D graph camera fit:
  - use explicit camera targeting to graph bounding-box center (load + engine stop)
  - compute camera distance from graph extent + camera FOV for deterministic framing
  - keep `forceX/forceY/forceZ` centering for stable layout evolution
  - keeps graph centered when panning is disabled
- [x] 2026-04-02: Extended `Makefile` to auto-install `gopls` locally when needed:
  - Added pinned tool vars: `GOPLS`, `GOPLS_VERSION`
  - Added `gopls` install target using existing `go-install-tool` pattern
  - Added `ensure-gopls` helper and wired it into `test-integration`
- [x] 2026-04-02: Updated `GOPLS_VERSION` pin to `v0.21.1` for Go 1.25 compatibility.
- [x] 2026-04-02: Improved server shutdown responsiveness on `Ctrl+C`:
  - Reduced HTTP graceful shutdown timeout from 30s to 5s
  - Added forced `http.Server.Close()` fallback when graceful shutdown times out
  - Prevents long hangs with active long-lived connections during termination
- [x] 2026-04-02: Completed Finding 2 scope-auth behavior coverage (TDD-first):
  - Added REST integration suite `internal/api/rest/scope_authz_integration_test.go` covering scope-taking write endpoints:
    - `POST /v1/memories`, `/v1/knowledge`, `/v1/skills`, `/v1/collections`, `/v1/knowledge/upload`, `/v1/sessions`
  - Added MCP integration suite `internal/api/mcp/scope_authz_integration_test.go` covering scope-taking tools:
    - `remember`, `publish`, `recall`, `context`, `skill_search`, `promote`, `session_begin`, `summarize`, `synthesize_topic`, `skill_install`, `skill_invoke`
    - `collect` action coverage: `create_collection`, `list_collections`, `add_to_collection` (slug+scope path)
  - Each test matrix asserts:
    - authorized scope -> success
    - unauthorized scope -> forbidden (`scope access denied`)
    - malformed scope -> bad request/tool error (`invalid scope`)
- [x] 2026-04-02: Completed Finding 3 ID-based scope authorization hardening (TDD-first):
  - Added object-scope helper in REST: `authorizeObjectScope(ctx, objectScopeID)`
  - Enforced object-scope checks on ID-based REST handlers:
    - memories: `GET/PATCH/DELETE /v1/memories/{id}`
    - knowledge: `GET/PATCH /v1/knowledge/{id}`
    - skills: `GET/PATCH /v1/skills/{id}`
    - collections: `GET /v1/collections/{id}`, `POST/DELETE /v1/collections/{id}/items...`
  - Added integration regression matrix in `internal/api/rest/scope_authz_integration_test.go`:
    - in-scope object IDs remain successful
    - out-of-scope object IDs return `403 forbidden: scope access denied`
- [x] 2026-04-02: Completed Finding 4 multi-hop principal chain API authorization (TDD-first):
  - Added effective-scope context cache primitives in `internal/api/scopeauth`:
    - `WithEffectiveScopeIDs`
    - `EffectiveScopeIDsFromContext`
  - `AuthorizeContextScope` now prefers cached effective scopes and only resolves via `MembershipStore.EffectiveScopeIDs` when cache is absent
  - Added REST middleware (`scopeAuthzContextMiddleware`) to preload effective scopes once per authenticated request
  - Added scopeauth unit test `TestAuthorizeContextScope_UsesCachedEffectiveScopes` verifying resolver bypass when cache is present
  - Added API integration matrix `TestREST_ScopeAuthz_MultiHopPrincipalChain`:
    - chain: `user -> team -> company`
    - roles: `member`, `owner`, `admin`
    - asserts allow self+ancestors and deny descendants + unrelated branch on scope-taking endpoint `POST /v1/sessions`
- [x] 2026-04-02: Completed Finding 5 memory fan-out authorization alignment (TDD-first):
  - Policy decision: use intersection model for recall safety (`fanOutScopes ∩ authorizedScopes`)
  - Added `RecallInput.AuthorizedScopeIDs` and intersect logic in `internal/memory/recall.go`
  - Added short-circuit when intersection is empty (no DB recall queries)
  - Wired authorized scope IDs into memory recall call sites:
    - REST: `/v1/memories/recall`, `/v1/context`
    - MCP: `recall`, `context`
  - Added tests:
    - unit: `TestRecall_IntersectAuthorizedScopeIDs`, `TestRecall_EmptyIntersectionSkipsDBQueries`
    - integration: `TestREST_Recall_IntersectsFanOutWithPrincipalScopes` (prevents ancestor-scope leakage)
- [x] 2026-04-02: Completed Finding 6 end-to-end API scope-auth security suite (TDD-first):
  - Added reusable multi-hop fixture graph helper:
    - `internal/testhelper.CreateScopeAuthzGraph(...)`
    - creates chain `user -> team -> company` and unrelated branch scopes/principals
  - Added REST end-to-end chain matrix:
    - `TestREST_ScopeAuthz_WriteEndpoints_MultiHopChainMatrix`
    - asserts positive (`user/team/company`) and negative (unrelated branch) for scope-taking REST write routes
  - Added MCP end-to-end chain matrix:
    - `TestMCP_ScopeAuthz_MultiHopChainMatrix`
    - asserts positive (`user/team/company`) and negative (unrelated branch) for scope-taking MCP tools
  - Added dedicated CI scope-auth gate in `.github/workflows/ci.yml`:
    - unit: `go test ./internal/api/scopeauth ./internal/memory`
    - integration: `go test -tags integration ./internal/api/rest ./internal/api/mcp -run "Test(REST|MCP)_ScopeAuthz_|TestREST_Recall_IntersectsFanOutWithPrincipalScopes"`
- [x] 2026-04-03: Aligned Scope Authz Gate with Make + JUnit outputs:
  - Added `test-scope-authz` and `test-scope-authz-integration` targets in `Makefile`.
  - Scope Authz CI job now runs those Make targets instead of raw `go test` commands.
  - Added Scope Authz Test Summary step reading `report-scope-authz-*.xml`.
  - Updated `.gitignore` from `report.xml` to `report*.xml` to ignore all generated JUnit reports.
- [x] 2026-04-02: Completed cross-cutting scope-auth observability and inventory tasks (TDD-first):
  - Added explicit REST/MCP scope-taking inventory tables used by inventory guard tests:
    - `restScopeRouteInventory` in `internal/api/rest/scopeauth_inventory_test.go`
    - `mcpScopeToolInventory` in `internal/api/mcp/scopeauth_inventory_test.go`
  - Added denied-attempt structured logging fields for both REST and MCP scope-auth failures:
    - `principal_id`, `requested_scope_id`, `token_id`, `endpoint`/`tool`
  - Added denied-attempt metric:
    - `postbrain_scope_authz_denied_total{surface,endpoint}` via `metrics.ScopeAuthzDenied`
  - Added observability unit tests:
    - `TestWriteScopeAuthzError_LogsFieldsAndIncrementsMetric`
    - `TestScopeAuthzToolError_LogsFieldsAndIncrementsMetric`
- [x] 2026-04-02: Added comprehensive principal scope-visibility integration matrix:
  - Table-driven coverage for principal chains: single-node (`user|team|department|company`) and multi-hop (`user->team`, `team->department`, `user->team->company`, up to `user->team->department->company`)
  - For each principal in chain, asserted `EffectiveScopeIDs` includes self+ancestors only (no descendants)
  - Added unrelated-branch exclusion assertion (no leakage of outsider scopes)
  - Added role variants (`member`, `owner`, `admin`) to confirm visibility inheritance is role-agnostic
- [x] 2026-04-02: Added scope owner reassignment (REST + UI, TDD-first):
  - REST: `PUT /v1/scopes/{id}/owner` with required `principal_id`
  - DB: new `UpdateScopeOwner` query + compat helper
  - UI: new `POST /ui/scopes/{id}/owner` handler and owner-change dialog/action on scopes page
  - Integration test extended to verify scope owner changes in `TestScopes_CRUD`
- [x] 2026-04-02: Redesigned `/ui/scopes` rows to multiline cards-in-table style for better readability without horizontal scrolling:
  - collapsed scope identity (name + `kind:external_id`) into stacked content
  - switched path/repository cells to wrapping monospace text
  - moved created/indexed info into stacked status lines
  - kept repository attach/edit/sync/delete actions, grouped with wrapping controls
- [x] 2026-04-02: Extended `Makefile` to auto-provision a dedicated markitdown venv for tests:
  - Added `ensure-markitdown` target used by both `test` and `test-integration`
  - Added `MARKITDOWN_VENV`, `MARKITDOWN_STAMP`, `MARKITDOWN_VERSION` variables
  - Test commands now prepend the venv `bin/` to `PATH`
  - Bootstrap installs pinned `markitdown[all]==0.1.5` so DOCX extraction tests have required extras
- [x] 2026-04-02: Fixed silently discarded errors (TDD: failing tests first):
  - `internal/memory/store.go`: log `slog.Warn` on `co_occurs_with` and code-graph `UpsertRelation` failures
  - `internal/knowledge/store.go`: log `slog.Warn` on `same_as` / `co_occurs_with` `UpsertRelation` failures
  - `internal/db/migrate.go`: capture `m.Version()` error as `verErr`; fix `errors.Is` checking wrong variable
  - `cmd/postbrain-cli/main.go`: add `parseSkillID` helper; use in `sync` and `install` to prevent zero-UUID DB writes
- [x] 2026-04-02: Implemented missing UI routes + templates (TDD: failing tests first):
  - `GET /ui/knowledge/new` → `handleKnowledgeNew` + `knowledge_new.html` template
  - `POST /ui/knowledge` → `handleCreateKnowledge` (validates title, scope_id; creates artifact)
  - `POST /ui/knowledge/:id/retract` → `handleKnowledgeRetract`
  - `POST /ui/memories/:id/forget` → `handleMemoryForget`
  - `GET /ui/collections/new` → `handleCollectionNew` + `collections_new.html` template
  - `POST /ui/collections` → `handleCreateCollection` (validates name, slug, scope_id)
  - Bug fix: `handleKnowledgeDetail` with nil pool now returns 404 instead of template-exec 500
- [x] 2026-04-02: Updated `designs/DESIGN_CODE_GRAPH.md` Phase 5a to prioritize chunk embedding by graph centrality (high in/out link degree first), with tie-breakers and caveat that degree is a strong but imperfect importance proxy.
- [x] 2026-04-02: Updated `designs/DESIGN_CODE_GRAPH.md` Phase 5b to split chunk embedding budget control into two caps:
  - `REPO_CHUNK_EMBED_MAX_GLOBAL` (fleet-wide/global window cap)
  - `REPO_CHUNK_EMBED_MAX_PER_REPO` (single-repo per-sync cap)
- [x] 2026-04-01: Memory API update (REST + MCP) for long-style preference and optional summary.
  - Added strict-TDD integration coverage for:
    - REST `/v1/memories` create + patch update persisting `summary`
    - MCP `remember` create + near-duplicate update persisting `summary`
    - default memory meta preference `{"content_style":"long"}` on create/update
  - Implemented request/schema/store/DB changes:
    - REST: `summary` added to create/update request payloads
    - MCP: `remember` accepts optional `summary` argument and tool schema documents it
    - Memory store: create/update now enforce `meta.content_style = "long"` and persist optional `summary`
    - DB query path: `UpdateMemoryContent` now updates `summary` and `meta`
  - Validation:
    - `go test -tags integration ./internal/api/rest ./internal/api/mcp` passed
    - `go test ./...` passes (parseSkillID undefined resolved in 2026-04-02 fix below).
- [x] 2026-04-01: Ran `make lint`, fixed all reported issues (errcheck/staticcheck/unused), then verified with `gofmt -w .`, `go test ./...`, and `make lint` (0 issues).
- [x] 2026-04-01: Added shared `internal/closeutil.Log` helper to report deferred close failures; replaced swallowed production `Close()` errors and added unit tests.

### Infrastructure & Bootstrap

- [x] `go.mod` — module `github.com/simplyblock/postbrain` with approved dependencies:
  - `github.com/jackc/pgx/v5` — PostgreSQL driver
  - `github.com/golang-migrate/migrate/v4` — schema migrations
  - `github.com/go-chi/chi/v5` — HTTP router
  - `github.com/spf13/viper` — config
  - `github.com/spf13/cobra` — CLI subcommands
  - `github.com/prometheus/client_golang` — metrics
  - `github.com/google/uuid` — UUID utilities
  - `log/slog` (stdlib) — structured logging
- [x] `scripts/postgres-init.sql` — installs pg_cron, pg_partman, vector as superuser
- [x] `docker-compose.yml` — services: `postgres` (pgvector/pgvector:pg18 with pg_cron + pg_partman), `ollama` (profile), `postbrain` (profile); volumes; health checks
- [x] `Makefile` — targets: `build`, `test`, `lint`, `fmt`, `migrate-up`, `migrate-down`, `docker-up`, `docker-down`, `generate`
- [x] `config.example.yaml` — complete reference file matching all keys in DESIGN.md Configuration section

### `internal/config` — Configuration

- [x] `config.go` — viper-based loader (TDD: test written first):
  - Load from file path, env vars (`POSTBRAIN_*`)
  - Validate required fields: `database.url`, `server.token`
  - Warn (slog) if `server.token == "changeme"`
  - Typed `Config` struct with all YAML keys; mapstructure tags
  - Tests: all fields, missing url, missing token, changeme warning, env override, defaults
- [x] `config_test.go` — 6 test cases, all passing

### `internal/db` — Database Layer

- [x] `conn.go` — pgx/v5 pool setup (TDD: test written first):
  - `MaxConns` from `database.max_open`, `MinConns` from `database.max_idle`
  - `ConnectTimeout` from `database.connect_timeout`
  - `AfterConnect` hook: `SET search_path = public`
  - `NewPool(ctx, cfg)` returns `*pgxpool.Pool`
- [x] `conn_test.go` — unit tests for invalid/empty URL

- [x] `migrate.go` — migration runner:
  - `//go:embed migrations/*.sql` to embed all SQL files
  - `ExpectedVersion` package-level const (set at build time via `-ldflags`)
  - `CheckAndMigrate(ctx, pool, cfg)`:
    1. Acquire PostgreSQL advisory lock (key `0x706f737462726169` — "postbrai" as int64)
    2. Check current schema version via golang-migrate
    3. If `current > ExpectedVersion`: log fatal "schema version N ahead of binary version M"
    4. If `dirty`: log fatal "schema is dirty at version N — run migrate force"
    5. Apply pending migrations (`migrate.Up()`)
    6. Release advisory lock
  - Expose `MigrateCmd` for the `postbrain migrate` subcommands (status, up, down N, version, force N)

- [x] `migrations/000001_initial_schema.up.sql` — 10 extensions, FTS config, embedding_models, touch_updated_at(), principals, tokens, principal_memberships, scopes, sessions, events (partitioned)
- [x] `migrations/000001_initial_schema.down.sql`
- [x] `migrations/000002_memory_graph.up.sql` — memories (6 indexes), entities, memory_entities, relations, triggers, pg_cron jobs (expire/decay/prune)
- [x] `migrations/000002_memory_graph.down.sql`
- [x] `migrations/000003_knowledge_layer.up.sql` — knowledge_artifacts, endorsements, history, collections, collection_items, sharing_grants, promotion_requests, staleness_flags, consolidations, forward FK memories.promoted_to, pg_cron stale-knowledge job, knowledge_status_idx
- [x] `migrations/000003_knowledge_layer.down.sql`
- [x] `migrations/000004_skills.up.sql` — skills, skill_endorsements, skill_history, triggers, events_skill_stats trigger
- [x] `migrations/000004_skills.down.sql`
- [x] `migrations/000005_age_graph.up.sql` — AGE graph setup wrapped in DO/EXCEPTION (no-op if absent)
- [x] `migrations/000005_age_graph.down.sql`
- [x] `migrations/000006_synthesis.up.sql` — `artifact_digest_sources (digest_id, source_id)` join table (bidirectional indexes), `knowledge_digest_log` audit table, `digest` added to `knowledge_type` enum
- [x] `migrations/000006_synthesis.down.sql`
- [x] `migrations/000007_artifact_graph.up.sql` — `artifact_entities (artifact_id, entity_id, role)` join table linking knowledge artifacts to entities; `source_artifact UUID` column on `relations` for knowledge-artifact provenance
- [x] `migrations/000007_artifact_graph.down.sql`

- [x] `models.go` — shared DB model types: `Memory`, `Principal`, `Membership`, `Scope`, `Token`, `Skill`, `SkillEndorsement`, `SkillHistory`, `SkillParameter`, `KnowledgeArtifact`, `KnowledgeEndorsement`, `KnowledgeHistory`, `KnowledgeCollection`, `KnowledgeCollectionItem`, `StalenessFlag`, `PromotionRequest`
- [x] `queries.go` — thin pgx query layer: `CreatePrincipal`, `GetPrincipalByID`, `GetPrincipalBySlug`, `CreateMembership`, `DeleteMembership`, `GetMemberships`, `GetAllParentIDs`, `CreateScope`, `GetScopeByID`, `GetScopeByExternalID`, `GetAncestorScopeIDs`, `CreateToken`, `LookupToken`, `RevokeToken`, `UpdateTokenLastUsed`; skill queries: `CreateSkill`, `GetSkill`, `GetSkillBySlug`, `UpdateSkillContent`, `UpdateSkillStatus`, `SnapshotSkillVersion`, `CreateSkillEndorsement`, `GetSkillEndorsementByEndorser`, `CountSkillEndorsements`, `RecallSkillsByVector`, `RecallSkillsByFTS`, `ListPublishedSkillsForAgent`; knowledge queries: `GetMemory`, `CreateArtifact`, `GetArtifact`, `UpdateArtifact`, `UpdateArtifactStatus`, `IncrementArtifactEndorsementCount`, `IncrementArtifactAccess`, `SnapshotArtifactVersion`, `CreateEndorsement`, `GetEndorsementByEndorser`, `ListVisibleArtifacts`, `RecallArtifactsByVector`, `RecallArtifactsByFTS`, `CreateCollection`, `GetCollection`, `GetCollectionBySlug`, `ListCollections`, `AddCollectionItem`, `RemoveCollectionItem`, `ListCollectionItems`, `InsertStalenessFlag`, `HasOpenStalenessFlag`, `UpdateStalenessFlag`, `CreatePromotionRequest`, `GetPromotionRequest`, `ListPendingPromotions`

- [x] sqlc layer — migrated from hand-written pgx queries to sqlc-generated code.
  `sqlc.yaml` and `internal/db/queries/*.sql` files define all queries. Generated files
  (`*.sql.go`, `models.go`, `db.go`) replace the old `queries.go`. A `compat.go` shim
  preserves the free-function API used by existing callers. Added `github.com/pgvector/pgvector-go`
  for vector type support. All callers updated to handle `pgvector.Vector` and `int32`/`time.Time`
  type changes. All 17 packages pass `go test ./...`.
  - Generated: `db/models.go`, `db/db.go`, `db/*.sql.go`
  - Added: `sqlc.yaml`, `internal/db/queries/`, `internal/db/compat.go`

### `internal/embedding` — Embedding Service

- [x] `interface.go` — `Embedder` interface:
  - `Embed(ctx context.Context, text string) ([]float32, error)`
  - `EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)`
  - `ModelSlug() string`
  - `Dimensions() int`
- [x] `classifier.go` — `ClassifyContent(content, sourceRef string) string` returns `"text"` or `"code"`:
  - If `source_ref` starts with `file:` and extension is in `{.go, .py, .js, .ts, .rs, .java, .c, .cpp, .h, .rb, .sh}` (and more): return `"code"`
  - Otherwise: count lines starting with common code patterns (braces, indentation ≥ 4 spaces, `func `, `def `, `class `); if ratio > 0.4: return `"code"`
  - Default: return `"text"`
- [x] `ollama.go` — Ollama HTTP backend:
  - POST `{ollama_url}/api/embeddings` with `{model, prompt}`
  - Respect `request_timeout` and `batch_size` from config
  - Return error if response `embedding` is empty
- [x] `openai.go` — OpenAI backend:
  - POST `https://api.openai.com/v1/embeddings` with `{model, input}`
  - Handle batch input (array of strings)
  - Respect `request_timeout` and `batch_size`
- [x] `service.go` — `EmbeddingService` wrapping text + code embedders:
  - `EmbedText(ctx, text) ([]float32, error)`
  - `EmbedCode(ctx, text) ([]float32, error)` — falls back to text model if no code model configured
  - `TextEmbedder() Embedder`, `CodeEmbedder() Embedder`
  - Supports ollama and openai backends; returns error for unknown backend

### `internal/principals` — Principal Management

- [x] `store.go`:
  - `Create(ctx, kind, slug, displayName, meta)` → Principal
  - `GetByID(ctx, id)`, `GetBySlug(ctx, slug)` → Principal
  - `Update(ctx, id, displayName, meta)`, `Delete(ctx, id)`
- [x] `store_test.go` — integration tests (build tag `integration`; skip if `TEST_DATABASE_URL` not set)
- [x] `membership.go`:
  - `AddMembership(ctx, memberID, parentID, role, grantedBy)`:
    - Before insert: run cycle check CTE; return `ErrCycleDetected` if a path from `parentID` back to `memberID` exists
    - Validate `role` is one of `"member"`, `"owner"`, `"admin"`
  - `RemoveMembership(ctx, memberID, parentID)`
  - `EffectiveScopeIDs(ctx, principalID) ([]uuid.UUID, error)` — recursive CTE from DESIGN.md
  - `IsScopeAdmin(ctx, principalID, scopeID) (bool, error)` — checks for `role='admin'` in own or ancestor scope
  - Sentinel errors: `ErrCycleDetected`, `ErrInvalidRole`
- [x] `membership_test.go` — unit tests for `ErrInvalidRole` and `ErrCycleDetected` logic (no DB required)

### `internal/auth` — Authentication Middleware

- [x] `tokens.go`:
  - `HashToken(raw string) string` — `hex(sha256([]byte(raw)))`
  - `GenerateToken() (raw, hash string, error)` — `crypto/rand` 32 bytes → hex → prepend `"pb_"` prefix
  - `TokenStore.Lookup` — enforces `revoked_at IS NULL`, `expires_at` check
  - `TokenStore.UpdateLastUsed` — fire-and-forget goroutine; do not block request path; nil pool = no-op
  - `EnforceScopeAccess(token *Token, requestedScopeID uuid.UUID) error` — if `token.ScopeIDs != nil`, reject if not in list
- [x] `tokens_test.go` — 7 unit tests (no DB required)
- [x] `middleware.go`:
  - `BearerTokenMiddleware(store, pool)` — extract `Authorization: Bearer <token>`, hash, lookup, attach `*Token` to context
  - Internal `tokenLookup` interface enables testing without a real DB
  - Return 401 JSON `{"error":"unauthorized"}` on missing/invalid/revoked token
  - Inject `ContextKeyToken`, `ContextKeyPrincipalID`, `ContextKeyPermissions` into request context
- [x] `middleware_test.go` — 4 httptest-based unit tests (no DB required)

### `internal/memory` — Memory Store

- [x] `store.go`:
  - `Create(ctx, input) (*Memory, error)`:
    1. Classify content_kind via `embedding.ClassifyContent(content, sourceRef)`
    2. Embed content with text model → set `embedding`, `embedding_model_id`
    3. If `content_kind = "code"`: also embed with code model → set `embedding_code`, `embedding_code_model_id`
    4. If `memory_type = "working"` and `expires_in > 0`: set `expires_at = now() + expires_in`; if `expires_in` omitted: default TTL = 3600s
    5. If `memory_type = "working"` and `expires_in` provided but type is not `"working"`: ignore `expires_in`
    6. Near-duplicate check: ANN query with cosine distance ≤ 0.05 in same scope; if match found: update existing row, return `action:"updated"`
    7. Insert memory row; insert `memory_entities` links if `entities` provided (upsert entity by canonical name)
    9. Return `{memory_id, action:"created"|"updated"}`
  - `Update(ctx, id, content, importance) (*Memory, error)` — re-embed on content change; bump version
  - `SoftDelete(ctx, id)` — set `is_active = false`
  - `HardDelete(ctx, id)` — DELETE row
  - Uses `memoryDB` interface for testability without a real DB

- [x] `recall.go`:
  - `Recall(ctx, input) ([]*MemoryResult, error)`:
    1. Resolve fan-out scopes via `FanOutScopeIDs`
    2. Based on `search_mode`:
       - `"text"`: ANN query on `embedding` column only
       - `"code"`: ANN query on `embedding_code` column
       - `"hybrid"` (default): HNSW vector + BM25 FTS + pg_trgm trigram similarity; merge by ID
    3. Apply combined score formula: `0.50*vec + 0.20*bm25 + 0.20*importance + 0.10*recency_decay`; trigram score (`similarity(content, query)`) blended in as additional signal
       - `recency_decay = exp(-λ * days_since_last_access)` where λ from memory_type (exported as `DecayLambda`)
    4. Deduplicate by `id`; apply `min_score` filter; sort DESC; truncate to `limit`
    5. Increment `access_count` + set `last_accessed` for returned rows (async, non-blocking goroutines)
    6. Append `{layer:"memory"}` to each result
  - Uses `recallDB` and `fanOutFunc` interfaces for testability

- [x] `scope.go`:
  - `FanOutScopeIDs(ctx, scopeID, principalID, maxDepth int, strictScope bool) ([]uuid.UUID, error)`:
    - Run ancestor CTE on ltree path
    - Add personal scope for principalID
    - TODO(task-scope-depth): implement maxDepth filtering using ltree path label counts
    - If `strict_scope = true`: return `[scopeID]` only (no fan-out)
  - `ResolveScopeByExternalID(ctx, kind, externalID) (*Scope, error)`

- [x] `consolidate.go`:
  - `FindClusters(ctx, scopeID) ([][]*Memory, error)`:
    - Fetch candidates: `is_active=true, importance < 0.7, access_count < 3`
    - Build similarity graph: cosine distance ≤ 0.05 between embeddings (union-find O(n²))
    - Return connected components as clusters (min cluster size: 2)
  - `MergeCluster(ctx, cluster []*Memory, summarizer func) (*Memory, error)`:
    1. Call injected summarizer with all cluster contents
    2. Create new memory with `memory_type = "semantic"`, merged content, `importance = max(cluster importances)`
    3. Soft-delete all source memories (`is_active = false`)
    4. Insert `consolidations` row with `strategy="merge"`, `source_ids`, `result_id`
    5. Return new memory
  - Uses `consolidatorDB` interface for testability

### `internal/knowledge` — Knowledge Layer

- [x] `store.go`:
  - `Create(ctx, input) (*KnowledgeArtifact, error)`:
    - Embed content with text model
    - Set `status = "draft"` unless `auto_review = true` → set `status = "in_review"`
    - Default `ReviewRequired = 1` if 0
    - Insert row; return artifact
  - `Update(ctx, id, callerID, title, content, summary) (*KnowledgeArtifact, error)`:
    - Only allowed when `status IN ("draft", "in_review")`; return `ErrNotEditable` for published/deprecated
    - Re-embed on content change; snapshot history for in_review
  - `GetByID(ctx, id)` — thin wrapper
  - Sentinel errors: `ErrNotEditable`, `ErrNotFound`
- [x] `lifecycle.go` — state machine from DESIGN.md:
  - `SubmitForReview`, `RetractToDraft`, `Endorse` (with auto-publish + idempotent duplicate handling), `Deprecate`, `Republish`, `EmergencyRollback`
  - Uses internal `lifecycleDB` interface for testability
  - Sentinel errors: `ErrSelfEndorsement`, `ErrNotReviewable`, `ErrForbidden`, `ErrInvalidTransition`
- [x] `visibility.go`:
  - `ResolveVisibleScopeIDs(ctx, pool, principalID, requestedScopeID) ([]uuid.UUID, error)`:
    - Walks ltree ancestor chain + adds personal scope
    - `deduplicateScopeIDs` helper (unit-tested)
- [x] `collections.go`:
  - `CollectionStore`: `Create`, `GetByID`, `GetBySlug`, `List`, `AddItem`, `RemoveItem`, `ListItems`
  - `Create` validates visibility against allowed set
- [x] `promote.go`:
  - `Promoter`: `CreateRequest` (with `ErrAlreadyPromoted` guard + mark nominated), `Approve` (5-step serializable tx), `Reject`
  - `PromoteInput` struct
- [x] `recall.go`:
  - `Recall(ctx, pool, input) ([]*ArtifactResult, error)`:
    - Hybrid: `RecallArtifactsByVector` + `RecallArtifactsByFTS`, merged by artifact ID
    - Score: `0.50*vec + 0.20*bm25 + 0.20*normalizeEndorsements(count) + 0.10*recency + 0.10 boost`
    - `RecallInput`, `ArtifactResult` types

### `internal/ingest` — Document Text Extraction

- [x] `extract.go` — `Extract(filename, data) (string, error)`: .txt/.md passthrough; PDF via `github.com/ledongthuc/pdf`; DOCX via zip+XML parse; `ErrUnsupportedFormat` sentinel
- [x] `extract_test.go` — TestExtractTxt, TestExtractMarkdown, TestExtractDocx, TestExtractUnsupported, TestExtractPDFInvalid

### `internal/skills` — Skills Registry

- [x] `store.go`:
  - `Create(ctx, input) (*Skill, error)` — embed `description + " " + body`; default `status = "draft"`
  - `Update(ctx, id, body, params) (*Skill, error)` — re-embed; snapshot to `skill_history`
  - `GetBySlug(ctx, scopeID, slug)`, `GetByID(ctx, id)`
- [x] `lifecycle.go` — identical state machine to knowledge:
  - `SubmitForReview`, `RetractToDraft`, `Endorse`, `AutoPublish`, `Deprecate`, `Republish`, `EmergencyRollback`
  - `Endorse`: reject if `endorserID == skill.author_id` → `ErrSelfEndorsement`
- [x] `recall.go`:
  - Hybrid retrieval on `description || ' ' || body` embedding + FTS
  - Filter: `status = "published"`, visibility resolved same as knowledge
  - Filter by `agent_types @> ARRAY[:agent_type] OR 'any' = ANY(agent_types)` if `agent_type` provided
  - Append `{layer:"skill"}` to each result
- [x] `install.go`:
  - `Install(ctx, skill *Skill, agentType, workdir string) (path string, error)`:
    - For `claude-code` or `any`: write to `{workdir}/.claude/commands/{slug}.md`
    - For `codex`: write to `{workdir}/.codex/skills/{slug}.md`
    - File format: YAML frontmatter (name, description, agent_types, parameters) + blank line + body
    - Overwrite existing file silently
    - Return absolute path of written file
  - `IsInstalled(slug, agentType, workdir string) bool` — checks file presence
- [x] `sync.go`:
  - `Sync(ctx, pool, scopeID, agentType, workdir string) (*SyncResult, error)`:
    - List all published skills for agent from DB
    - For each: check `IsInstalled`; if not or outdated (version mismatch in frontmatter): install
    - For installed files not in registry (or deprecated): report as `orphaned`
    - Return `{installed, updated, orphaned []string}`
- [x] `invoke.go`:
  - `Invoke(ctx, skillID, params map[string]any) (string, error)`:
    - Validate all `required: true` params present
    - Validate types: `string`, `integer`, `boolean`, `enum` (check values list)
    - On validation failure: return `ErrValidation{Fields: [{name, reason}]}`
    - Substitute `$PARAM_NAME` and `{{param_name}}` in body (both syntaxes)
    - Return expanded body string

### `internal/retrieval` — Cross-layer Retrieval

- [x] `merge.go`:
  - `CombineScores(vecScore, bm25Score, importance, recencyDecay float64, layer Layer) float64`
    - w_vec=0.50, w_bm25=0.20, w_imp=0.20, w_rec=0.10; LayerKnowledge adds +0.1 boost
  - `Merge(results []*Result, limit int, minScore float64) []*Result`
    - Deduplication: drops memory results whose PromotedTo is in the knowledge result set
    - Filters by minScore, sorts by score DESC, truncates to limit
  - `RecallInput` and `Result` unified type for cross-layer retrieval
  - TODO: full concurrent multi-layer Recall function using errgroup

### `internal/sharing` — Sharing Grants

- [x] `grants.go`:
  - `Create(ctx, g) (*Grant, error)` — exactly one of `memory_id` or `artifact_id` must be set (ErrInvalidGrant)
  - `Revoke(ctx, grantID)` — deletes grant by ID
  - `List(ctx, granteeScopeID, limit, offset)` — paginated listing
  - `IsMemoryAccessible(ctx, memoryID, requesterScopeID) (bool, error)`
  - `IsArtifactAccessible(ctx, artifactID, requesterScopeID) (bool, error)`

### `internal/graph` — Entity & Relation Graph

- [x] `relations.go`:
  - `UpsertEntity(ctx, scopeID, entityType, name, canonical, meta) (*Entity, error)`
  - `UpsertRelation(ctx, scopeID, subjectID, predicate, objectID, confidence, sourceMemoryID) (*Relation, error)`
  - `LinkMemoryToEntity(ctx, memoryID, entityID, role string)` — validates role; ErrInvalidRole for invalid values
  - `ListRelationsForEntity(ctx, entityID, predicate string)` — predicate="" means all
  - `ExtractEntitiesFromMemory(content string, sourceRef *string) []*Entity` — file: paths, pr:NNN, PascalCase concepts (heuristic fallback)
- [x] `internal/knowledge/store.go` — `analyzeContent` helper: attempts LLM-based combined summarise+entity-extract call (JSON: `{summary, entities[{name,type,canonical}]}`); falls back silently to extractive summary (`knowledge.Summarize`) + heuristic entity extraction (`graph.ExtractEntitiesFromMemory`); LLM errors never block writes
- [x] `internal/knowledge/store.go` — `artifact_entities` linking: for each extracted entity, calls `db.LinkArtifactToEntity` with role `"related"`; connects same-canonical entities in the same scope with `same_as` relations (subject/object order normalised to lower UUID first to prevent duplicates)
- [x] `internal/db/compat.go` — `ListEntitiesByCanonical`, `LinkArtifactToEntity` helper functions supporting the same_as linking path

- [x] `age_sync.go` — (optional, skipped if AGE unavailable):
  - `SyncEntityToAGE(ctx, pool, entity *Entity) error` — MERGE vertex by id property
  - `SyncRelationToAGE(ctx, pool, rel *Relation) error` — MERGE edge by (subject, predicate, object)
- [x] `age_query.go` — (optional):
  - `RunCypherQuery(ctx, pool, scopeID, cypher string) ([]map[string]any, error)` — prepend scope filter to Cypher
  - Return `ErrAGEUnavailable` if AGE not detected
- [x] `pagerank.go` — (optional):
  - Weekly job: compute PageRank over the AGE graph for the `relations` edge set
  - Write scores back to `entities.meta["pagerank"]`

### `internal/jobs` — Background Jobs

- [x] `scheduler.go`:
  - `robfig/cron` setup; one cron per enabled job flag
  - Each job logs start/end/duration via slog with job name
  - Each job recovers from panics and logs error without crashing
  - `age_check_enabled` logs that pg_cron handles `detect-stale-knowledge-age`
- [x] `expire.go` — on-demand or scheduled TTL cleanup (supplement to pg_cron working-memory expiry)
- [x] `consolidate.go`:
  - Run every 6 hours
  - Per scope: find clusters (cosine ≤ 0.05), call `memory.MergeCluster` for each cluster with ≥ 2 members
  - Respect `jobs.consolidation_enabled` config flag
  - `defaultSummarizer` fallback (no LLM): joins with `\n---\n`
- [x] `reembed.go`:
  - Fetches active text/code model from `embedding_models`; skips if none registered
  - Processes in batches of `batchSize` (default 64)
  - Text and code model jobs run independently (`RunText`, `RunCode`)
  - Sets `embedding_model_id` / `embedding_code_model_id` after successful embed
  - TODO(task-jobs): insert reembed_batch events once background job scope handling is defined
- [x] `staleness.go` — Signal 2 weekly contradiction detection:
  1. Fetch all published knowledge artifacts in batches (100/batch)
  2. For each artifact: fetch recent memories from last 7 days in same/ancestor scopes
  3. Pre-filter by cosine similarity > 0.6 (topic overlap)
  4. Apply negation pre-filter: similarity to `"[title] is false/wrong/outdated"` > 0.5
  5. For survivors: call LLM; classify as CONTRADICTS / CONSISTENT / UNRELATED
  6. If CONTRADICTS and no open flag exists: insert `staleness_flags` row
  7. `confidence = min(0.9, negation_similarity * 1.5)`
  - `noopClassifier` safe default for deployments without LLM
- [x] `promotion_notify.go`:
  - Logs pending promotion requests; TODO(task-jobs) for webhook/email notification
- [x] `retrieval.CosineSimilarity` — added exported helper to `internal/retrieval/merge.go`

### `internal/api/mcp` — MCP Server

- [x] `server.go` — mcp-go SSE server; Bearer token auth middleware; all 13 tools registered
  - TODO(task-mcp-sessions): create sessions row on connect; update ended_at on disconnect
  - TODO(task-mcp-age): detect AGE availability and conditionally register advanced graph tools
- [x] `remember.go` — delegates to `memory.Store.Create`; maps `expires_in`, entities
- [x] `recall.go` — delegates to memory/knowledge/skill stores; maps `search_mode`, `layers`, `min_score`
- [x] `forget.go` — soft or hard delete; returns `{memory_id, action}`
- [x] `context.go` — knowledge first (greedy max_tokens), then memories; returns `{context_blocks, total_tokens}`
- [x] `summarize.go` — `dry_run` support; simple join summarizer (TODO: LLM-based)
- [x] `publish.go` — `auto_review` flag; delegates to `knowledge.Store.Create`
- [x] `endorse.go` — tries knowledge.Lifecycle.Endorse then skills.Lifecycle.Endorse; returns `{endorsement_count, status, auto_published}`
- [x] `promote.go` — delegates to `knowledge.Promoter.CreateRequest`
- [x] `collect.go` — dispatches on `action`: `add_to_collection`, `create_collection`, `list_collections`
- [x] `skill_search.go` — delegates to `skills.Store.Recall`
- [x] `skill_install.go` — delegates to `skills.Install`
- [x] `skill_invoke.go` — delegates to `skills.Invoke`; returns expanded body
- [x] `graph_query.go` — scoped Cypher traversal via AGE overlay; returns `rows` JSON payload
- [x] `knowledge_detail.go` — returns full artifact content by ID; used when `recall` returns `full_content_available=true`
- [x] `server_test.go` — 3+ table-driven tests (handleForget shape, handleRemember missing content, handleRecall layer filter)

### `internal/api/rest` — REST API

- [x] `router.go` — chi router; `BearerTokenMiddleware` on all /v1 routes; all routes registered
  - TODO(task-rest-cors): CORS headers configurable via config
- [x] `memories.go` — `POST /v1/memories`, `POST /v1/memories/summarize`, `GET /v1/memories/recall`, `GET/PATCH/DELETE /v1/memories/:id`, `POST /v1/memories/:id/promote`
- [x] `knowledge.go` — `POST /v1/knowledge`, `GET /v1/knowledge/search`, `GET/PATCH /v1/knowledge/:id`, `POST /v1/knowledge/:id/endorse`, `POST /v1/knowledge/:id/deprecate`, `GET /v1/knowledge/:id/history`
- [x] `collections.go` — `POST /v1/collections`, `GET /v1/collections`, `GET /v1/collections/:slug`, `POST/DELETE /v1/collections/:id/items`
- [x] `skills.go` — `POST /v1/skills`, `GET /v1/skills/search`, `GET/PATCH /v1/skills/:id`, `POST /v1/skills/:id/endorse`, `POST /v1/skills/:id/deprecate`, `POST /v1/skills/:id/install`, `POST /v1/skills/:id/invoke`
- [x] `sharing.go` — `POST /v1/sharing/grants`, `DELETE /v1/sharing/grants/:id`, `GET /v1/sharing/grants`
- [x] `promotions.go` — `GET /v1/promotions`, `POST /v1/promotions/:id/approve`, `POST /v1/promotions/:id/reject`
- [x] `scopes.go` — `GET/POST /v1/scopes`, `GET/PUT/DELETE /v1/scopes/:id`; DB layer extended with `ListScopes`, `UpdateScope`, `DeleteScope`; web UI principals page updated to show scopes table
- [x] Fix nil-UUID FK violations on `knowledge_artifacts` and `skills` inserts (`previous_version`, `source_memory_id`, `source_artifact_id` — add `NULLIF` guards matching scopes pattern)
- [x] Fix nullable `TIMESTAMPTZ` columns scanned into non-pointer `time.Time` fields (crash on first publish/remember): change `PublishedAt`, `DeprecatedAt`, `LastAccessed`, `ExpiresAt`, `LastInvokedAt`, `EndedAt`, `ReviewedAt` to `*time.Time` across all models, generated sql.go files, and callers
- [x] `orgs.go` — `GET/POST /v1/principals`, `GET/PUT/DELETE /v1/principals/:id`, `GET/POST/DELETE /v1/principals/:id/members`
- [x] `sessions.go` — `POST /v1/sessions`, `PATCH /v1/sessions/:id` (TODO: persist to DB)
- [x] `context.go` — `GET /v1/context`
- [x] `health.go` — `GET /health`
- [x] `helpers.go` — `writeJSON`, `writeError`, `readJSON`, `uuidParam`, `paginationFromRequest`
- [x] `router_test.go` — GET /health 200, POST /v1/memories no auth 401, invalid token 401
- [x] `upload.go` + `upload_test.go` — `POST /v1/knowledge/upload` multipart file upload; text extraction via `internal/ingest`; 401 test; supports .txt, .md, .pdf, .docx
- [x] `graph.go` — `GET /v1/entities?scope_id=&type=&limit=&offset=`, `GET /v1/graph?scope_id=`, and `POST /v1/graph/query` implemented (AGE-aware: returns 501 when AGE unavailable)

### `cmd/postbrain` — Server Binary

- [x] `main.go`:
  - cobra root command with `--config` flag
  - `serve` subcommand: load config → `db.NewPool` → optional `CheckAndMigrate` → `embedding.NewService` → MCP + REST mux → TLS-capable `net.Listen` → graceful shutdown on SIGINT/SIGTERM
  - `migrate` subcommand with sub-subcommands: `up` (wired), `down [N]` (TODO stub), `status` (TODO stub), `version` (TODO stub), `force <N>` (TODO stub)
  - Prometheus `/metrics` via `promhttp.Handler()`
  - Background job scheduler started via `jobs.NewScheduler`

### `cmd/postbrain-cli` — Hook CLI

- [x] `main.go` — cobra dispatch; reads `POSTBRAIN_URL`, `POSTBRAIN_TOKEN` from env; `--scope` persistent flag
- [x] `snapshot` subcommand:
  - Reads Claude Code PostToolUse JSON from stdin
  - Extracts `file_path`/`path` from `tool_input` as `source_ref`
  - Dedup check via `/v1/memories/recall?min_score=0.99`; skips if hit
  - POSTs to `/v1/memories` with `memory_type="episodic"`, `scope` from `--scope` flag
- [x] `summarize-session` subcommand:
  - Fetches episodic memories via REST; skips if count < 3
  - POSTs to `POST /v1/memories/summarize` (REST endpoint added to router)
  - `--session` flag (defaults to `CLAUDE_SESSION_ID` env)
- [x] `skill sync` subcommand:
  - GETs `/v1/skills/search?status=published`; calls `skills.Install` for new skills
  - Reports orphaned local `.claude/commands/*.md` files not in registry
- [x] `skill install` subcommand: fetches by slug, TODO parse+install (stub)
- [x] `skill list` subcommand: globs local `.claude/commands/*.md`
- [x] `POST /v1/memories/summarize` REST endpoint added to `internal/api/rest/memories.go` and registered in router

---

## Testing Tasks

All tests follow TDD: test file written before implementation file.

### Unit Tests

- [x] `internal/config` — valid config loads; missing required fields return error; `"changeme"` token logs warning; env var overlay overrides YAML values (6 tests in `config_test.go`)
- [x] `internal/auth/tokens.go` — `HashToken` is deterministic; `GenerateToken` produces `"pb_"` prefix; scope enforcement rejects out-of-scope requests; expired token rejected (8 tests in `tokens_test.go` + `middleware_test.go`)
- [x] `internal/embedding/classifier.go` — Go source file classified as `"code"`; prose text classified as `"text"`; file with unknown extension falls back to content heuristic (`classifier_test.go`)
- [x] `internal/memory/recall.go` — combined score formula produces correct values for known inputs; `min_score` filter excludes low-scoring results; `strict_scope=true` returns only the target scope (10 tests in `recall_test.go`)
- [x] `internal/retrieval/merge.go` — promoted memory is deduplicated when knowledge artifact is also present; knowledge boost of +0.1 applied; results sorted DESC by score (5 tests in `merge_test.go`)
- [x] `internal/skills/invoke.go` — `$PARAM_NAME` substituted correctly; `{{param_name}}` substituted correctly; missing required param returns `ErrValidation`; wrong enum value returns `ErrValidation`; integer type validation rejects string value (7 tests in `invoke_test.go`)
- [x] `internal/knowledge/lifecycle.go` — self-endorsement returns `ErrSelfEndorsement`; auto-publish fires when `endorsement_count >= review_required`; `Deprecate` rejects non-admin caller; `EmergencyRollback` clears `published_at` and `deprecated_at` (7 tests in `lifecycle_test.go`)
- [x] `internal/principals/membership.go` — cycle detection rejects A→B→A; direct self-loop rejected by DB constraint; `IsScopeAdmin` returns true for ancestor-scope admin (5 tests in `membership_test.go`)

### Integration Tests (require real PostgreSQL via testcontainers)

- [x] `internal/db` — migrations apply cleanly; `MigrateForTest` strips pg_cron/pg_partman/pg_prewarm and downsizes vector dims to 4; key tables verified after migration; `internal/db/migrate_test_helpers.go` + `migrate_integration_test.go`
- [x] Memory lifecycle — `Create` returns `action:"updated"` for near-duplicate (cosine ≤ 0.05 with deterministic embedder); `Create` with `memory_type="working"` sets `expires_at ≈ now()+3600s`; `SoftDelete` excludes from `Recall`; `HardDelete` removes row; `memory_integration_test.go`
- [x] Scope fan-out — querying a project scope returns memories from project, team, department, and company scopes; `strict_scope=true` returns only exact scope; `scope_integration_test.go`
- [x] Knowledge promotion workflow — nomination creates pending request; approval transaction creates artifact, sets `promoted_to` and `promotion_status="promoted"` atomically; re-nomination of already-nominated memory is rejected; `promote_integration_test.go`
- [x] Knowledge endorsement → auto-publish — artifact reaches `review_required` endorsements and transitions to `published`; self-endorsement rejected; `lifecycle_integration_test.go`
- [x] Staleness flags — `source_modified` flag inserted via Go; `HasOpenStalenessFlag` detects the open flag; different signal not detected; `staleness_integration_test.go`
- [x] Skill install/invoke — `Install` writes correct frontmatter + body to `.claude/commands/<slug>.md`; `Invoke` with valid params returns expanded body; `Invoke` with missing required param returns `*ValidationError`; `skills_integration_test.go`
- [x] Shared test infrastructure — `internal/testhelper/container.go` (`NewTestPool` via testcontainers pgvector/pgvector:pg18); `fixtures.go` (`CreateTestPrincipal`, `CreateTestScope`, `CreateTestEmbeddingModel`); `embedding.go` (`NewMockEmbeddingService`, `NewDeterministicEmbeddingService`)
- [x] Fixed pre-existing compile error in `internal/principals/store_test.go` (`*db.Pool` → `*pgxpool.Pool`)

### E2E Tests

- [x] MCP tool calls via mcp-go test client — `remember` → `recall` → `forget` round-trip; `publish` → `endorse` → auto-publish flow; `skill_search` returns installed flag correctly (`mcp_integration_test.go`, build tag `integration`)
- [x] REST API via `net/http/httptest` — all CRUD endpoints return correct status codes; unauthenticated request returns 401; health returns 200 (`rest_integration_test.go`, build tag `integration`)

---

## Observability Tasks

- [x] Prometheus metrics (`internal/metrics/metrics.go`):
  - `postbrain_tool_duration_seconds{tool}` histogram — p50/p99 per MCP tool (instrumented in `internal/api/mcp/server.go`)
  - `postbrain_embedding_duration_seconds{backend,model}` histogram
  - `postbrain_job_duration_seconds{job}` histogram (instrumented in `internal/jobs/scheduler.go`)
  - `postbrain_active_memories_total{scope}` gauge (updated on write/delete)
  - `postbrain_recall_results_total{layer}` counter
- [x] `log/slog` structured logging — `requestLoggerMiddleware` in `internal/api/rest/logging.go` injects `request_id` and `principal_id` into every /v1 request context; `LogFromContext` helper for handlers
- [x] `/health` endpoint: `{"status":"ok","schema_version":N,"expected_version":M,"schema_dirty":false}` — returns 503 if dirty or version mismatch (`internal/api/rest/health.go` + `internal/db/schema_version.go`)

---

## Web UI Tasks

Technology: Go `html/template` + HTMX + Pico.css, all embedded via `//go:embed`. Served at `/ui` from the existing binary. See designs/DESIGN.md § Web UI for full specification.

### Prerequisites

- [x] `internal/api/rest/graph.go` — `GET /v1/entities`, `GET /v1/graph`, `POST /v1/graph/query` (required by the entity graph page; implement before the graph page)

### Infrastructure

- [x] `web/static/pico.min.css` — embed Pico.css v2 (classless theme; placeholder file — replace with real minified file from picocss.com)
- [x] `web/static/htmx.min.js` — embed HTMX v2 (placeholder file — replace with real minified file from htmx.org)
- [x] `web/templates/base.html` — shared layout: `<nav>`, `<main>`, `<footer>`; active-page highlighting; HTMX boosted links
- [x] `internal/ui/handler.go` — `NewHandler(pool, svc)` returns `http.Handler`; `//go:embed web/templates` + `//go:embed web/static`; template rendering helpers (`render`); `POST /ui/knowledge/upload` via `handleUploadKnowledge`
- [x] `internal/ui/auth.go` — session cookie middleware (`pb_session`); `?token=` query-param on `/ui/login` only; calls `auth.TokenStore.Lookup`; 401 → redirect to `/ui/login`
- [x] `internal/ui/handler_test.go` — unit tests: unauthenticated request redirects to login; login form sets cookie; template renders without panicking
- [x] `cmd/postbrain/main.go` — mount `ui.NewHandler` at `/ui` and `/ui/` in `runServe`

### Pages

- [x] **Login** (`web/templates/login.html`) — token input form; POST to `/ui/login`; sets `pb_session` cookie; redirects to `/ui`
- [x] **Overview** (`web/templates/health.html`) — server status badge, schema version
- [x] **Memory Browser** (`web/templates/memories.html` + `memories_rows.html` partial) — scope selector; search bar; paginated results table; soft-delete button (HTMX swap)
- [x] **Memory Detail** (`web/templates/memory_detail.html`) — full content, metadata, promote button
- [x] **Knowledge Browser** (`web/templates/knowledge.html` + `knowledge_rows.html` partial) — filter by status; inline endorse buttons; HTMX row swap; document upload form (.txt/.md/.pdf/.docx)
- [x] **Knowledge Detail** (`web/templates/knowledge_detail.html`) — content pane, endorsement count
- [x] **Collections** (`web/templates/collections.html`) — collection list; links use UUID (`/ui/collections/{id}`)
- [x] **Collection Detail** (`web/templates/collection_detail.html`) — artifact table with links to knowledge detail; back link; route `GET /ui/collections/{id}`
- [x] **Promotion Queue** (`web/templates/promotions.html`) — pending requests table; approve/reject via `POST /ui/promotions/{id}/approve|reject` (cookie-auth plain forms, redirect back)
- [x] **Staleness Flags** (`web/templates/staleness.html`) — open flags table
- [x] **Entity Graph** (`web/templates/graph.html`) — entity and relation tables for scope; data from `GET /v1/graph`
- [x] **Skills Registry** (`web/templates/skills.html`) — published skills list
- [x] **Principals & Scopes** (`web/templates/principals.html`) — principals table, scopes table with delete button, memberships table with remove button, create-principal form, create-scope form, add-membership form (member/parent/role dropdowns); `POST /ui/principals`, `POST /ui/scopes`, `POST /ui/memberships`, `POST /ui/memberships/delete` routes wired in `ServeHTTP`; `ListAllMemberships` query added to `db/compat.go`
- [x] **Scopes** (`web/templates/scopes.html`) — flat list of all scopes with external ID, ltree path, and delete button; repository attach/edit + sync actions; responsive table layout with grouped action controls; route `GET /ui/scopes`
- [x] **Metrics** (`web/templates/metrics.html`) — Prometheus metrics reference page

#### Missing / Planned Screens

- [x] **Skill Detail** (`web/templates/skill_detail.html`) — full skill body, parameters table, invocation count, last invoked; endorse button; route `GET /ui/skills/{id}`
- [x] **Skill History** (`web/templates/skill_history.html`) — version changelog table; route `GET /ui/skills/{id}/history`; backed by `db.GetSkillHistory` (hand-written compat query)
- [x] **Knowledge History** (`web/templates/knowledge_history.html`) — version timeline; diff viewer; route `GET /ui/knowledge/{id}/history`; backed by `GET /v1/knowledge/{id}/history`
- [ ] **Staleness Flag Resolution** — add resolve/dismiss action to existing `promotions.html` staleness view; `POST /ui/staleness/{id}/resolve` and `/dismiss` with optional review note; backed by `db.ResolveStalenessFlag`
- [x] **Token Management** (`web/templates/tokens.html`) — list tokens with name, last used, expiry, status; create-token form (name, scope multi-select, expiry); revoke button; raw token shown once after creation; routes `GET /ui/tokens`, `POST /ui/tokens`, `POST /ui/tokens/{id}/revoke`; backed by `auth.TokenStore` + `db.ListTokens`
- [ ] **Sharing Grants** (`web/templates/sharing.html`) — list active grants (what, to whom, expiry, can-reshare); create-grant form (select artifact, grantee scope, reshare policy, expiry); revoke button; route `GET /ui/sharing`; backed by `GET /v1/sharing/grants`
- [ ] **Session List** (`web/templates/sessions.html`) — list sessions with scope, principal, start time, duration (or "active"); route `GET /ui/sessions`; backed by `sessions` table
- [ ] **Session Detail** (`web/templates/session_detail.html`) — session metadata; chronological event log (event type, payload, timestamp) from `events` table; route `GET /ui/sessions/{id}`
- [ ] **Consolidation History** (`web/templates/consolidations.html`) — list consolidation records per scope; source memory IDs, result memory link, strategy, reason; route `GET /ui/consolidations`; backed by `consolidations` table
- [ ] **Entity Detail** (`web/templates/entity_detail.html`) — entity name, type, canonical, linked memories, outgoing/incoming relations; route `GET /ui/graph/entities/{id}`; backed by `db.GetEntity`, `db.ListRelationsByScope`
- [ ] **Embedding Models** (`web/templates/models.html`) — list registered embedding models (slug, dimensions, content type, active flag); route `GET /ui/models`; backed by `db.GetActiveTextModel` / `embedding_models` table

### Testing

- [x] Unit tests (`internal/ui/handler_test.go`) — NewHandler(nil) succeeds; login GET returns 200; unauthenticated /ui redirects; unauthenticated /ui/metrics redirects
- [ ] Integration tests (`internal/ui/ui_integration_test.go`, build tag `integration`) — login flow, memory browser returns 200, unauthenticated redirect, promote action returns correct status

---

## TUI Tasks

Technology: `bubbletea` + `lipgloss` + `bubbles` (Charmbracelet suite). New binary `postbrain-tui`. Communicates via REST API only — no direct DB access. See designs/DESIGN.md § TUI for full specification.

### New dependencies (add to `go.mod`)

- [ ] `github.com/charmbracelet/bubbletea` — Elm-architecture TUI framework
- [ ] `github.com/charmbracelet/lipgloss` — layout and colour primitives
- [ ] `github.com/charmbracelet/bubbles` — list, text input, spinner, paginator, viewport components

### REST client

- [ ] `internal/tui/client.go` — `Client{baseURL, token string; http *http.Client}`; typed methods for every REST endpoint used by the TUI:
  - `RecallMemories(ctx, scopeID, query string, limit int) ([]*MemoryResult, error)`
  - `GetMemory(ctx, id string) (*db.Memory, error)`
  - `SoftDeleteMemory(ctx, id string) error`
  - `HardDeleteMemory(ctx, id string) error`
  - `PromoteMemory(ctx, id, targetScope, visibility string) error`
  - `ListKnowledge(ctx, scopeID, status string) ([]*db.KnowledgeArtifact, error)`
  - `GetArtifact(ctx, id string) (*db.KnowledgeArtifact, error)`
  - `EndorseArtifact(ctx, id string) error`
  - `DeprecateArtifact(ctx, id string) error`
  - `ListPromotions(ctx string) ([]*db.PromotionRequest, error)`
  - `ApprovePromotion(ctx, id string) error`
  - `RejectPromotion(ctx, id string) error`
  - `ListSkills(ctx, scopeID, agentType string) ([]*db.Skill, error)`
  - `InvokeSkill(ctx, id string, params map[string]any) (string, error)`
  - `ListEntities(ctx, scopeID string) ([]*db.Entity, error)`
  - `ListRelations(ctx, scopeID string) ([]*db.Relation, error)`
- [ ] `internal/tui/client_test.go` — unit tests using `httptest.Server` mock; verify each method serialises requests and deserialises responses correctly

### Model and screen stack

- [ ] `internal/tui/model.go` — top-level `Model` implementing `tea.Model`; screen stack (`[]tea.Model`); global key dispatch (`?` help, `q`/`Esc` pop screen, `ctrl+c` quit); window-resize message propagation
- [ ] `internal/tui/model_test.go` — unit tests for screen push/pop, quit key, resize propagation (no HTTP)

### Screens

- [ ] `internal/tui/screens/scopes.go` — scope selector; fetches principal's scopes from `GET /v1/principals/{id}`; renders as indented tree using lipgloss; `Enter` pushes memory list screen
- [ ] `internal/tui/screens/memories.go` — memory list (bubbles `list.Model`); `/` opens search input; `Enter` pushes detail; `d` soft-delete with confirm prompt; `D` hard-delete with confirm; `p` opens promote form; `q` pops
- [ ] `internal/tui/screens/knowledge.go` — knowledge list (bubbles `list.Model`); `/` search; `Enter` detail; `e` endorse; `dep` deprecate; `q` pops; status badge coloured via lipgloss
- [ ] `internal/tui/screens/promotions.go` — promotion queue (bubbles `list.Model`); `a` approve; `x` reject; confirmation prompt before each action; `q` pops
- [ ] `internal/tui/screens/skills.go` — skills list; `i` install (prompts agent type via text input); `inv` opens param form (one input per required param); rendered result shown in viewport; `q` pops
- [ ] `internal/tui/screens/graph.go` — ASCII adjacency list of entities and relations from `GET /v1/graph`; lipgloss borders; `/` searches and highlights matching node; `Enter` shows all relations for selected entity; `q` pops
- [ ] `internal/tui/screens/help.go` — full keybinding reference rendered in a lipgloss table; `q` or `?` dismisses

### Binary

- [ ] `cmd/postbrain-tui/main.go` — cobra root command; `--config` (reads `server.addr` and `server.token`); `--url` override; `--token` override; `POSTBRAIN_URL` / `POSTBRAIN_TOKEN` env var fallback; `run` subcommand starts bubbletea program

### Testing

- [ ] `internal/tui/screens/*_test.go` — unit tests for each screen's `Update` function using synthetic `tea.Msg` values; no HTTP required
- [ ] Integration test (`internal/tui/tui_integration_test.go`, build tag `integration`) — full round-trip via httptest server: scope select → memory list → create memory via REST → verify it appears in TUI list model
