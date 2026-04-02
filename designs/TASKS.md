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

### Maintenance

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
  - `cmd/postbrain-hook/main.go`: add `parseSkillID` helper; use in `sync` and `install` to prevent zero-UUID DB writes
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

- [ ] `age_sync.go` — (optional, skipped if AGE unavailable):
  - `SyncEntityToAGE(ctx, pool, entity *Entity) error` — MERGE vertex by id property
  - `SyncRelationToAGE(ctx, pool, rel *Relation) error` — MERGE edge by (subject, predicate, object)
  - `DetectAGE(ctx, pool) bool` — `SELECT * FROM ag_catalog.ag_graph LIMIT 1`; return false on error
- [ ] `age_query.go` — (optional):
  - `RunCypherQuery(ctx, pool, scopeID, cypher string) ([]map[string]any, error)` — prepend scope filter to Cypher
  - Return `ErrAGEUnavailable` if AGE not detected
- [ ] `pagerank.go` — (optional):
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
  - TODO(task-mcp-age): detect AGE availability and conditionally register graph tools
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
- [x] `graph.go` — `GET /v1/entities?scope_id=&type=&limit=&offset=` and `GET /v1/graph?scope_id=` implemented; `POST /v1/graph/query` returns 501 (AGE unavailable — deferred)

### `cmd/postbrain` — Server Binary

- [x] `main.go`:
  - cobra root command with `--config` flag
  - `serve` subcommand: load config → `db.NewPool` → optional `CheckAndMigrate` → `embedding.NewService` → MCP + REST mux → TLS-capable `net.Listen` → graceful shutdown on SIGINT/SIGTERM
  - `migrate` subcommand with sub-subcommands: `up` (wired), `down [N]` (TODO stub), `status` (TODO stub), `version` (TODO stub), `force <N>` (TODO stub)
  - Prometheus `/metrics` via `promhttp.Handler()`
  - Background job scheduler started via `jobs.NewScheduler`

### `cmd/postbrain-hook` — Hook CLI

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
