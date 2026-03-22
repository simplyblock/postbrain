# Postbrain — Implementation Task List

## Design & Documentation (COMPLETE)

- [x] Three-layer model design (Memory, Knowledge, Skills)
- [x] Principal and scope hierarchy model
- [x] Database schema (all tables, indexes, triggers, pg_cron jobs)
- [x] MCP tool specifications (remember, recall, forget, context, summarize, publish, endorse, promote, collect, skill_search, skill_install, skill_invoke)
- [x] REST API specification (all endpoints, pagination)
- [x] Hybrid retrieval strategy (HNSW ANN + BM25 FTS + scoring formula)
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

### Infrastructure & Bootstrap

- [ ] `go.mod` — initialize module `github.com/noctarius/postbrain` with approved dependencies:
  - `github.com/jackc/pgx/v5` — PostgreSQL driver
  - `github.com/golang-migrate/migrate/v4` — schema migrations
  - `github.com/go-chi/chi/v5` — HTTP router
  - `github.com/mark3labs/mcp-go` — MCP server
  - `github.com/robfig/cron/v3` — in-process job scheduler
  - `gopkg.in/yaml.v3` + `github.com/spf13/viper` — config
  - `github.com/spf13/cobra` — CLI subcommands
  - `github.com/prometheus/client_golang` — metrics
  - `github.com/google/uuid` — UUID utilities
  - `log/slog` (stdlib, Go 1.21+) — structured logging
- [ ] `docker-compose.yml` — services: `postgres` (pgvector image with pg_cron + pg_partman), `ollama`; volumes for data persistence; health checks
- [ ] `Makefile` — targets: `build`, `test`, `lint` (golangci-lint), `fmt` (gofmt), `migrate-up`, `migrate-down`, `docker-up`, `docker-down`, `generate` (sqlc)
- [ ] `config.example.yaml` — complete reference file matching all keys in DESIGN.md Configuration section

### `internal/config` — Configuration

- [ ] `config.go` — viper-based loader:
  - Load from file path (flag `--config`), then `~/.config/postbrain/config.yaml`, then env vars (`POSTBRAIN_DATABASE_URL`, etc.)
  - Validate required fields: `database.url`, `server.token`
  - Warn if `server.token == "changeme"`
  - Expose typed `Config` struct matching all YAML keys: `Database`, `Embedding`, `Server`, `Migrations`, `Jobs`
  - `Jobs` sub-struct: `ConsolidationEnabled`, `ContradictionEnabled`, `ReembedEnabled`, `AgeCheckEnabled`

### `internal/db` — Database Layer

- [ ] `conn.go` — pgx/v5 pool setup:
  - `MaxConns` from `database.max_open`, `MinConns` from `database.max_idle`
  - `ConnectTimeout` from `database.connect_timeout`
  - `AfterConnect` hook: `SET search_path = public, ag_catalog` (supports AGE if present)
  - Expose `Pool` and `Close()`

- [ ] `migrate.go` — migration runner:
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

- [ ] `migrations/000001_initial_schema.up.sql` — in this exact order:
  1. `CREATE EXTENSION` for all 10 extensions (vector, pg_trgm, btree_gin, ltree, citext, unaccent, fuzzystrmatch, pg_prewarm, pg_cron, pg_partman)
  2. `CREATE TEXT SEARCH CONFIGURATION postbrain_fts`
  3. `CREATE TABLE embedding_models` + both partial unique indexes
  4. `CREATE TABLE principals` + `touch_updated_at` trigger for principals
  5. `CREATE TABLE tokens` + all 3 indexes (including GIN on scope_ids)
  6. `CREATE TABLE principal_memberships` + parent index
  7. `CREATE TABLE scopes` + all 3 indexes + `scopes_compute_path` trigger
  8. `CREATE OR REPLACE FUNCTION touch_updated_at()`
  9. `CREATE TABLE sessions`
  10. `CREATE TABLE events` (partitioned) + both indexes + `partman.create_parent` call + retention config
  11. `events_skill_stats` trigger function + trigger (even though skills table comes later — function body references skills table by name only, so it compiles; the trigger itself is added in migration 000005)

- [ ] `migrations/000001_initial_schema.down.sql` — drop all objects from 000001 in reverse order

- [ ] `migrations/000002_memory_graph.up.sql` — in this exact order:
  1. `CREATE TABLE memories` + all 5 indexes
  2. `CREATE TABLE entities` + HNSW index
  3. `CREATE TABLE memory_entities` — note: `role CHECK (role IN ('subject','object','context','related'))`
  4. `CREATE TABLE relations` + subject/object indexes
  5. `touch_updated_at` triggers for memories, entities
  6. `ALTER TABLE memories ADD CONSTRAINT memories_promoted_to_fk` — forward FK placeholder (references knowledge_artifacts; knowledge_artifacts does NOT exist yet — add the FK in migration 000003 instead, not here)
  7. pg_cron jobs: `expire-working-memory` (*/5), `decay-memory-importance` (nightly 03:00), `prune-low-value-memories` (Sunday 04:00)

- [ ] `migrations/000002_memory_graph.down.sql`

- [ ] `migrations/000003_knowledge_layer.up.sql` — in this exact order:
  1. `CREATE TABLE knowledge_artifacts` + all 4 indexes
  2. `CREATE TABLE knowledge_endorsements`
  3. `CREATE TABLE knowledge_history`
  4. `CREATE TABLE knowledge_collections` + `touch_updated_at` trigger
  5. `CREATE TABLE knowledge_collection_items`
  6. `CREATE TABLE sharing_grants` + all 3 indexes
  7. `CREATE TABLE promotion_requests` + both indexes
  8. `CREATE TABLE staleness_flags` + both indexes
  9. `CREATE TABLE consolidations`
  10. `touch_updated_at` trigger for knowledge_artifacts
  11. `ALTER TABLE memories ADD CONSTRAINT memories_promoted_to_fk FOREIGN KEY (promoted_to) REFERENCES knowledge_artifacts(id) ON DELETE SET NULL` — forward FK now possible
  12. pg_cron job: `detect-stale-knowledge-age` (monthly 1st 06:00)
  13. `CREATE INDEX knowledge_status_idx` (draft/in_review filter index)

- [ ] `migrations/000003_knowledge_layer.down.sql`

- [ ] `migrations/000004_skills.up.sql` — in this exact order:
  1. `CREATE TABLE skills` + all 3 indexes
  2. `CREATE TABLE skill_endorsements`
  3. `CREATE TABLE skill_history`
  4. `touch_updated_at` trigger for skills
  5. `CREATE OR REPLACE FUNCTION skills_update_invocation_stats()` + `CREATE TRIGGER events_skill_stats` on events table
  6. Note in migration comment: trigger on parent partitioned table; requires PG13+

- [ ] `migrations/000004_skills.down.sql`

- [ ] `migrations/000005_age_graph.up.sql` — wrapped entirely in `DO $$ BEGIN ... EXCEPTION WHEN ... END $$` so it is a no-op if AGE is not installed:
  - `CREATE EXTENSION IF NOT EXISTS age`
  - `SELECT create_graph('postbrain')`
  - `CREATE VLABEL entity`, `CREATE ELABEL relation`
  - Note: migration is idempotent; skips silently if AGE absent

- [ ] `migrations/000005_age_graph.down.sql`

- [ ] `db/queries/memories.sql` — sqlc queries:
  - `CreateMemory`, `GetMemory`, `UpdateMemory`, `SoftDeleteMemory`, `HardDeleteMemory`
  - `IncrementAccessCount` (also sets `last_accessed = now()`)
  - `RecallByVector` (HNSW ANN, scope-filtered, `is_active = true`)
  - `RecallByFTS` (BM25 via `ts_rank_cd`, scope-filtered)
  - `RecallByCodeVector` (uses `embedding_code`, `WHERE embedding_code IS NOT NULL`)
  - `FanOutScopes` (ancestor CTE using ltree `@>`)
  - `ListExpiredWorkingMemories`, `DecayImportance`, `PruneMemories`
  - `ListConsolidationCandidates` (importance < 0.7, access_count < 3, is_active = true)

- [ ] `db/queries/knowledge.sql` — sqlc queries:
  - `CreateArtifact`, `GetArtifact`, `UpdateArtifact`
  - `TransitionStatus` (takes from_status, to_status, principal_id for auth check)
  - `AddEndorsement`, `GetEndorsementCount`, `CheckSelfEndorsement`
  - `SnapshotVersion` (insert into knowledge_history)
  - `ListByVisibility` (ltree visibility query from DESIGN.md)
  - `IncrementArtifactAccess`
  - `ListPendingReview`, `ListOpenStalenessFlags`
  - `InsertStalenessFlag`, `UpdateStalenessFlag`

- [ ] `db/queries/collections.sql` — sqlc queries:
  - `CreateCollection`, `GetCollection`, `ListCollections`
  - `AddItemToCollection`, `RemoveItemFromCollection`, `ListCollectionItems`

- [ ] `db/queries/principals.sql` — sqlc queries:
  - `CreatePrincipal`, `GetPrincipalByID`, `GetPrincipalBySlug`
  - `CreateMembership`, `DeleteMembership`, `GetMemberships`
  - `EffectiveScopeIDs` (recursive CTE from DESIGN.md)
  - `CreateToken`, `LookupToken` (by token_hash), `RevokeToken`, `UpdateTokenLastUsed`
  - `GetScopeByKindAndExternalID`, `CreateScope`

- [ ] `db/queries/scopes.sql` — sqlc queries:
  - `GetScopeByID`, `GetScopeByPath`, `CreateScope`, `UpdateScope`
  - `AncestorScopeIDs` (ltree `@>` CTE)
  - `DescendantScopeIDs` (ltree `<@` for cascade recompute)

- [ ] `db/queries/sharing.sql` — sqlc queries:
  - `CreateGrant`, `RevokeGrant`, `ListGrantsForScope`
  - `IsMemoryGrantedToScope`, `IsArtifactGrantedToScope`

- [ ] `db/queries/skills.sql` — sqlc queries:
  - `CreateSkill`, `GetSkill`, `GetSkillBySlug`, `UpdateSkill`
  - `AddSkillEndorsement`, `CheckSkillSelfEndorsement`, `GetSkillEndorsementCount`
  - `SnapshotSkillVersion`
  - `RecallSkillsByVector`, `RecallSkillsByFTS`
  - `ListPublishedSkillsForAgent` (filters by agent_types contains agent OR 'any')

- [ ] `db/queries/promotions.sql` — sqlc queries:
  - `CreatePromotionRequest`, `GetPromotionRequest`
  - `ApprovePromotion` (sets status='approved', result_artifact_id)
  - `RejectPromotion`
  - `ListPendingPromotions`

- [ ] `db/queries/graph.sql` — sqlc queries:
  - `UpsertEntity`, `GetEntityByCanonical`, `ListEntitiesByScope`
  - `UpsertRelation`, `ListRelationsForEntity`
  - `LinkMemoryToEntity`, `ListEntitiesForMemory`

- [ ] `sqlc.yaml` — sqlc config pointing to all query files and the migration schema files

### `internal/embedding` — Embedding Service

- [ ] `interface.go` — `Embedder` interface:
  - `Embed(ctx context.Context, text string) ([]float32, error)`
  - `EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)`
  - `ModelSlug() string`
  - `Dimensions() int`
- [ ] `classifier.go` — `ClassifyContent(content, sourceRef string) string` returns `"text"` or `"code"`:
  - If `source_ref` starts with `file:` and extension is in `{.go, .py, .js, .ts, .rs, .java, .c, .cpp, .h, .rb, .sh}`: return `"code"`
  - Otherwise: count lines starting with common code patterns (braces, indentation ≥ 4 spaces, `func `, `def `, `class `); if ratio > 0.4: return `"code"`
  - Default: return `"text"`
- [ ] `ollama.go` — Ollama HTTP backend:
  - POST `{ollama_url}/api/embeddings` with `{model, prompt}`
  - Respect `request_timeout` and `batch_size` from config
  - Return error if response `embedding` is empty
- [ ] `openai.go` — OpenAI backend:
  - POST `https://api.openai.com/v1/embeddings` with `{model, input}`
  - Handle batch input (array of strings)
  - Respect `request_timeout` and `batch_size`
- [ ] `service.go` — `EmbeddingService` wrapping text + code embedders:
  - `EmbedText(ctx, text) ([]float32, modelID, error)`
  - `EmbedCode(ctx, text) ([]float32, modelID, error)` — falls back to text model if no code model configured
  - `ActiveTextModelID() uuid.UUID`
  - `ActiveCodeModelID() uuid.UUID`
  - Validates at startup: active model dimensions match the `vector(N)` column size (queries `embedding_models`)

### `internal/principals` — Principal Management

- [ ] `store.go`:
  - `Create(ctx, kind, slug, displayName, meta)` → Principal
  - `GetByID(ctx, id)`, `GetBySlug(ctx, slug)` → Principal
  - `Update(ctx, id, displayName, meta)`, `Delete(ctx, id)`
- [ ] `membership.go`:
  - `AddMembership(ctx, memberID, parentID, role, grantedBy)`:
    - Before insert: run cycle check CTE; return `ErrCycleDetected` if a path from `parentID` back to `memberID` exists
    - Validate `role` is one of `"member"`, `"owner"`, `"admin"`
  - `RemoveMembership(ctx, memberID, parentID)`
  - `EffectiveScopeIDs(ctx, principalID) ([]uuid.UUID, error)` — recursive CTE from DESIGN.md
  - `IsScopeAdmin(ctx, principalID, scopeID) (bool, error)` — checks for `role='admin'` in own or ancestor scope

### `internal/auth` — Authentication Middleware

- [ ] `tokens.go`:
  - `HashToken(raw string) string` — `hex(sha256([]byte(raw)))`
  - `GenerateToken() (raw, hash string, error)` — `crypto/rand` 32 bytes → hex → prepend `"pb_"` prefix
  - `LookupToken(ctx, pool, hash) (*Token, error)` — enforces `revoked_at IS NULL`, `expires_at` check
  - `UpdateLastUsed(pool, tokenID)` — fire-and-forget goroutine; do not block request path
  - `EnforceScopeAccess(token *Token, requestedScopeID uuid.UUID) error` — if `token.ScopeIDs != nil`, reject if not in list
- [ ] `middleware.go`:
  - `BearerTokenMiddleware(pool)` — extract `Authorization: Bearer <token>`, hash, lookup, attach `*Token` to context
  - Return 401 JSON `{"error":"unauthorized"}` on missing/invalid/revoked token
  - Inject `ContextKeyPrincipalID`, `ContextKeyPermissions` into request context

### `internal/memory` — Memory Store

- [ ] `store.go`:
  - `Create(ctx, input) (*Memory, error)`:
    1. Classify content_kind via `embedding.ClassifyContent(content, sourceRef)`
    2. Embed content with text model → set `embedding`, `embedding_model_id`
    3. If `content_kind = "code"`: also embed with code model → set `embedding_code`, `embedding_code_model_id`
    4. If `memory_type = "working"` and `expires_in > 0`: set `expires_at = now() + expires_in`; if `expires_in` omitted: default TTL = 3600s
    5. If `memory_type = "working"` and `expires_in` provided but type is not `"working"`: ignore `expires_in`
    6. Near-duplicate check: ANN query with cosine distance ≤ 0.05 in same scope; if match found: update existing row, return `action:"updated"`
    7. Insert memory row; insert `memory_entities` links if `entities` provided (upsert entity by canonical name)
    8. Insert `events` row `{event_type:"memory_write", payload:{memory_id}}`
    9. Return `{memory_id, action:"created"|"updated"}`
  - `Update(ctx, id, content, importance) (*Memory, error)` — re-embed on content change; bump version
  - `SoftDelete(ctx, id)` — set `is_active = false`
  - `HardDelete(ctx, id)` — DELETE row; only if caller has write permission on owning scope

- [ ] `recall.go`:
  - `Recall(ctx, input) ([]*MemoryResult, error)`:
    1. Resolve fan-out scopes via `FanOutScopes` CTE (ancestor scopes + personal scope + sharing grants)
    2. Based on `search_mode`:
       - `"text"` or `"hybrid"`: ANN query on `embedding` column
       - `"code"` or `"hybrid"`: ANN query on `embedding_code` column (skip if no code embedding for query)
    3. FTS BM25 query (always, combined in hybrid)
    4. Apply combined score formula: `0.50*vec + 0.20*bm25 + 0.20*importance + 0.10*recency_decay`
       - `recency_decay = exp(-λ * days_since_last_access)` where λ from memory_type
    5. Deduplicate by `id`; apply `min_score` filter; sort DESC; truncate to `limit`
    6. Increment `access_count` + set `last_accessed` for returned rows (async, non-blocking)
    7. Append `{layer:"memory"}` to each result

- [ ] `scope.go`:
  - `FanOutScopeIDs(ctx, scopeID, principalID, maxDepth int) ([]uuid.UUID, error)`:
    - Run ancestor CTE on ltree path
    - Add personal scope for principalID
    - Optionally cap depth via `max_scope_depth` param
    - If `strict_scope = true`: return `[scopeID]` only (no fan-out)
  - `ResolveScopeByExternalID(ctx, kind, externalID) (*Scope, error)`

- [ ] `consolidate.go`:
  - `FindClusters(ctx, scopeID) ([][]*Memory, error)`:
    - Fetch candidates: `is_active=true, importance < 0.7, access_count < 3`
    - Build similarity graph: cosine distance ≤ 0.05 between embeddings
    - Return connected components as clusters (min cluster size: 2)
  - `MergeCluster(ctx, cluster []*Memory) (*Memory, error)`:
    1. Call LLM summarizer with all cluster contents
    2. Create new memory with `memory_type = "semantic"`, merged content, `importance = max(cluster importances)`
    3. Soft-delete all source memories (`is_active = false`)
    4. Insert `consolidations` row with `strategy="merge"`, `source_ids`, `result_id`
    5. Return new memory

### `internal/knowledge` — Knowledge Layer

- [ ] `store.go`:
  - `Create(ctx, input) (*Artifact, error)`:
    - Embed content with text model
    - Set `status = "draft"` unless `auto_review = true` → set `status = "in_review"`
    - Insert row; return artifact
  - `Update(ctx, id, content, title, summary) (*Artifact, error)`:
    - Only allowed when `status IN ("draft", "in_review")`; return `ErrNotEditable` for published/deprecated
    - Re-embed on content change
    - If `status = "published"`: snapshot to `knowledge_history` first, then update and increment `version`
  - `GetByID(ctx, id)`, `Search(ctx, query, scopeIDs, visibility)`

- [ ] `recall.go`:
  - Same hybrid formula as memory recall
  - Filters: `status = "published"`, visibility resolved via ltree query (from DESIGN.md)
  - Apply `+0.1` importance boost over raw formula (institutional trust boost)
  - Append `{layer:"knowledge"}` to each result

- [ ] `visibility.go`:
  - `ResolveVisibleArtifacts(ctx, principalID, scopeID) ([]uuid.UUID, error)`:
    - Use ltree visibility query from DESIGN.md
    - Include artifacts shared via `sharing_grants` to any of the principal's scopes
    - Exclude `status != "published"` from read results (draft/in_review are private to author + scope admins)

- [ ] `lifecycle.go` — enforce the state machine from DESIGN.md exactly:
  - `SubmitForReview(ctx, artifactID, callerID)`:
    - Allowed from: `draft` only
    - Who: author OR scope admin
    - Action: set `status = "in_review"`
  - `RetractTodraft(ctx, artifactID, callerID)`:
    - Allowed from: `in_review` only
    - Who: author OR scope admin
    - Action: set `status = "draft"`
  - `Endorse(ctx, artifactID, endorserID, note)`:
    - Reject if `endorserID == artifact.author_id` → return `ErrSelfEndorsement`
    - Reject if artifact not `in_review` → return `ErrNotReviewable`
    - Insert `knowledge_endorsements` row (unique constraint handles duplicates)
    - Increment `endorsement_count` on artifact
    - If `endorsement_count >= review_required`: call `AutoPublish`
    - Return `{endorsement_count, status, auto_published}`
  - `AutoPublish(ctx, artifactID)` (internal):
    - Set `status = "published"`, `published_at = now()`
    - Snapshot to `knowledge_history`
  - `Deprecate(ctx, artifactID, callerID)`:
    - Who: scope admin only → verify via `principals.IsScopeAdmin`
    - Set `status = "deprecated"`, `deprecated_at = now()`
  - `Republish(ctx, artifactID, callerID)`:
    - Allowed from: `deprecated` only
    - Who: scope admin only
    - Set `status = "published"`, clear `deprecated_at`
  - `EmergencyRollback(ctx, artifactID, callerID)`:
    - Who: scope admin only
    - Set `status = "draft"`, clear `published_at` and `deprecated_at`

- [ ] `collections.go`:
  - `CreateCollection(ctx, scopeID, ownerID, slug, name, visibility) (*Collection, error)`
  - `GetCollection(ctx, id)`, `GetCollectionBySlug(ctx, scopeID, slug)`
  - `ListCollections(ctx, scopeID, principalID)`
  - `AddItem(ctx, collectionID, artifactID, addedBy)` — appends at max(position)+1
  - `RemoveItem(ctx, collectionID, artifactID)`

- [ ] `promote.go` — 5-step atomic promotion transaction:
  1. Validate: memory `promotion_status != "promoted"` and `!= "nominated"` (idempotency guard)
  2. Create `knowledge_artifacts` row with `status = "draft"`, `source_memory_id = memory.id`
  3. Set `promotion_requests.result_artifact_id`, `status = "approved"`
  4. Set `memories.promoted_to = artifact.id`, `promotion_status = "promoted"`
  5. Commit — all in one `pgx.Tx`
  - `RejectPromotion(ctx, requestID, reviewerID, note)`: set `status = "rejected"`, record reviewer/note

### `internal/skills` — Skills Registry

- [ ] `store.go`:
  - `Create(ctx, input) (*Skill, error)` — embed `description + " " + body`; default `status = "draft"`
  - `Update(ctx, id, body, params) (*Skill, error)` — re-embed; snapshot to `skill_history`
  - `GetBySlug(ctx, scopeID, slug)`, `GetByID(ctx, id)`
- [ ] `lifecycle.go` — identical state machine to knowledge:
  - `SubmitForReview`, `RetractToDraft`, `Endorse`, `AutoPublish`, `Deprecate`, `Republish`, `EmergencyRollback`
  - `Endorse`: reject if `endorserID == skill.author_id` → `ErrSelfEndorsement`
- [ ] `recall.go`:
  - Hybrid retrieval on `description || ' ' || body` embedding + FTS
  - Filter: `status = "published"`, visibility resolved same as knowledge
  - Filter by `agent_types @> ARRAY[:agent_type] OR 'any' = ANY(agent_types)` if `agent_type` provided
  - Append `{layer:"skill"}` to each result
- [ ] `install.go`:
  - `Install(ctx, skill *Skill, agentType, workdir string) (path string, error)`:
    - For `claude-code` or `any`: write to `{workdir}/.claude/commands/{slug}.md`
    - For `codex`: write to `{workdir}/.codex/skills/{slug}.md`
    - File format: YAML frontmatter (name, description, agent_types, parameters) + blank line + body
    - Overwrite existing file silently
    - Return absolute path of written file
  - `IsInstalled(slug, agentType, workdir string) bool` — checks file presence
- [ ] `sync.go`:
  - `Sync(ctx, pool, scopeID, agentType, workdir string) (*SyncResult, error)`:
    - List all published skills for agent from DB
    - For each: check `IsInstalled`; if not or outdated (version mismatch in frontmatter): install
    - For installed files not in registry (or deprecated): report as `orphaned`
    - Return `{installed, updated, orphaned []string}`
- [ ] `invoke.go`:
  - `Invoke(ctx, skillID, params map[string]any) (string, error)`:
    - Validate all `required: true` params present
    - Validate types: `string`, `integer`, `boolean`, `enum` (check values list)
    - On validation failure: return `ErrValidation{Fields: [{name, reason}]}`
    - Substitute `$PARAM_NAME` and `{{param_name}}` in body (both syntaxes)
    - Return expanded body string

### `internal/retrieval` — Cross-layer Retrieval

- [ ] `merge.go` — `Recall(ctx, input) ([]*Result, error)`:
  1. Concurrently query memory, knowledge, skills layers (goroutines + `errgroup`)
  2. Apply per-layer scores (memory: raw formula; knowledge: raw + 0.1 boost; skill: raw formula)
  3. Deduplicate: if a memory has `promoted_to` pointing to a knowledge artifact that is also in results, keep the knowledge artifact and drop the memory
  4. Merge all results into single slice; sort by `score DESC`
  5. Apply `limit` and `min_score` cutoffs
  6. Each result carries `layer` field: `"memory"`, `"knowledge"`, or `"skill"`

### `internal/sharing` — Sharing Grants

- [ ] `grants.go`:
  - `CreateGrant(ctx, input) (*Grant, error)` — exactly one of `memory_id` or `artifact_id` must be set; enforce CHECK at app layer too
  - `RevokeGrant(ctx, grantID, callerID)` — only grantor or scope admin can revoke
  - `ListGrantsForScope(ctx, granteeScoped, limit, offset)`
  - `IsAccessible(ctx, memoryID *uuid.UUID, artifactID *uuid.UUID, requesterScopeID uuid.UUID) (bool, error)`:
    - Check direct scope ownership, visibility level, or active unexpired grant

### `internal/graph` — Entity & Relation Graph

- [ ] `relations.go`:
  - `UpsertEntity(ctx, scopeID, entityType, name, canonical, meta) (*Entity, error)`
  - `UpsertRelation(ctx, scopeID, subjectID, predicate, objectID, confidence, sourceMemoryID) (*Relation, error)`
  - `LinkMemoryToEntity(ctx, memoryID, entityID, role string)` — role must be one of `subject`, `object`, `context`, `related`
  - `ListRelationsForEntity(ctx, entityID, predicate string)` — predicate="" means all
  - `ExtractEntitiesFromMemory(ctx, content, sourceRef string) ([]*Entity, error)` — heuristic: extract file paths from `source_ref`, named tokens from content using simple NER
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

- [ ] `scheduler.go`:
  - `robfig/cron` setup; one cron per enabled job flag
  - Each job logs start/end/duration via slog with job name
  - Each job recovers from panics and logs error without crashing
- [ ] `expire.go` — on-demand or scheduled TTL cleanup (supplement to pg_cron working-memory expiry)
- [ ] `consolidate.go`:
  - Run every 6 hours
  - Per scope: find clusters (cosine ≤ 0.05), call `memory.MergeCluster` for each cluster with ≥ 2 members
  - Respect `jobs.consolidation_enabled` config flag
- [ ] `reembed.go`:
  - Triggered on startup if active model ID differs from any row's `embedding_model_id`
  - Runs in background goroutine; processes in batches of `embedding.batch_size`
  - Text and code model jobs run independently
  - Logs `reembed_batch` events to `events` table on each batch completion
  - Sets `embedding_model_id` after successful embed
- [ ] `staleness.go` — Signal 2 weekly contradiction detection:
  1. Fetch all published knowledge artifacts in batches (100/batch)
  2. For each artifact: fetch recent memories from last 7 days in same/ancestor scopes
  3. Pre-filter by cosine similarity > 0.6 (topic overlap)
  4. Apply negation pre-filter: similarity to `"[title] is false/wrong/outdated"` > 0.5
  5. For survivors: call LLM; classify as CONTRADICTS / CONSISTENT / UNRELATED
  6. If CONTRADICTS and no open flag exists: insert `staleness_flags` row
  7. `confidence = min(0.9, negation_similarity * 1.5)`
- [ ] `promotion_notify.go`:
  - Runs every hour; finds promotion_requests where `status = "pending"` and no notification sent in last 24h
  - Notifies reviewer: log to slog (webhook/email out of scope for MVP; use TODO marker)

### `internal/api/mcp` — MCP Server

- [ ] `server.go` — mcp-go server setup:
  - SSE transport on `/mcp`
  - Bearer token auth on SSE connection request (same middleware as REST)
  - Create `sessions` row on connect; update `ended_at` on disconnect
  - Register all 13 tools
  - Detect AGE availability at startup; conditionally register graph tools
- [ ] `remember.go` — delegates to `memory.Store.Create`; maps `expires_in`, entities
- [ ] `recall.go` — delegates to `retrieval.Merge.Recall`; maps all input params including `search_mode`, `layers`, `min_score`
- [ ] `forget.go` — soft or hard delete; returns `{memory_id, action}`
- [ ] `context.go`:
  - Query knowledge first (greedy until `max_tokens` budget used), then memories
  - Omit entire block if it would exceed budget (no truncation)
  - Return `{context_blocks, total_tokens}`
- [ ] `summarize.go` — `dry_run` support; returns full output schema for both modes
- [ ] `publish.go` — `auto_review` flag; delegates to `knowledge.Store.Create`
- [ ] `endorse.go` — delegates to `knowledge.Lifecycle.Endorse` or `skills.Lifecycle.Endorse`; returns `{endorsement_count, status, auto_published}`
- [ ] `promote.go` — delegates to `knowledge.Promote.CreateRequest`
- [ ] `collect.go` — dispatches on `action`: `add_to_collection`, `create_collection`, `list_collections`
- [ ] `skill_search.go` — delegates to `skills.Recall`; passes `installed` filter (resolved locally)
- [ ] `skill_install.go` — delegates to `skills.Install`
- [ ] `skill_invoke.go` — delegates to `skills.Invoke`; returns expanded body

### `internal/api/rest` — REST API

- [ ] `router.go`:
  - chi router; `BearerTokenMiddleware` on all routes
  - Pagination middleware: parse `limit`, `offset`, `cursor` from query params; inject into context
  - All routes registered (see handler files below)
  - CORS headers configurable via config
- [ ] `memories.go` — `POST /v1/memories`, `GET /v1/memories/recall`, `GET/PATCH/DELETE /v1/memories/:id`, `POST /v1/memories/:id/promote`
- [ ] `knowledge.go` — `POST /v1/knowledge`, `GET /v1/knowledge/search`, `GET/PATCH /v1/knowledge/:id`, `POST /v1/knowledge/:id/endorse`, `POST /v1/knowledge/:id/deprecate`, `GET /v1/knowledge/:id/history`; `GET /v1/staleness`
- [ ] `collections.go` — `POST /v1/collections`, `GET /v1/collections`, `GET /v1/collections/:slug`, `POST/DELETE /v1/collections/:id/items`
- [ ] `skills.go` — `POST /v1/skills`, `GET /v1/skills/search`, `GET/PATCH /v1/skills/:id`, `POST /v1/skills/:id/endorse`, `POST /v1/skills/:id/deprecate`, `POST /v1/skills/:id/install`, `POST /v1/skills/:id/invoke`, `GET /v1/skills/:id/history`, `GET /v1/skills/:id/stats`
- [ ] `sharing.go` — `POST /v1/sharing/grants`, `DELETE /v1/sharing/grants/:id`, `GET /v1/sharing/grants`
- [ ] `promotions.go` — `GET /v1/promotions`, `POST /v1/promotions/:id/approve`, `POST /v1/promotions/:id/reject`
- [ ] `orgs.go` — `GET/POST /v1/principals`, `GET/PUT/DELETE /v1/principals/:id`, `GET/POST/DELETE /v1/principals/:id/members`; legacy `/v1/orgs` alias via chi `Mount`
- [ ] `sessions.go` — `POST /v1/sessions`, `PATCH /v1/sessions/:id`
- [ ] `graph.go` — `GET /v1/entities`, `GET /v1/graph`, `POST /v1/graph/query` (returns `graph_unavailable:true` if AGE absent)
- [ ] `context.go` — `GET /v1/context`

### `cmd/postbrain` — Server Binary

- [ ] `main.go`:
  - cobra root command with `--config` flag
  - `serve` subcommand: load config → `CheckAndMigrate` → start MCP + REST servers
  - `migrate` subcommand with sub-subcommands: `status`, `up`, `down <N>`, `version`, `force <N>`
  - `health` subcommand: print `{status, schema_version, expected_version}` and exit

### `cmd/postbrain-hook` — Hook CLI

- [ ] `main.go` — cobra dispatch; reads `POSTBRAIN_URL`, `POSTBRAIN_TOKEN` from env; falls back to config file at `$POSTBRAIN_CONFIG` or `~/.config/postbrain/config.yaml`
- [ ] `snapshot.go`:
  - Read tool output JSON from stdin (Claude Code PostToolUse hook format)
  - Extract: tool name, modified file paths, brief description
  - 60s dedup check: skip if a memory for same `source_ref` was created in last 60s (query REST API)
  - POST to `/v1/memories` with `memory_type="episodic"`, `source_ref=<file>`, `scope` from `--scope` flag
- [ ] `summarize_session.go`:
  - Fetch episodic memories for current session ID (from `CLAUDE_SESSION_ID` env or `--session` flag)
  - Skip if count < 3
  - POST to `/v1/memories/summarize` (or MCP `summarize` tool)
- [ ] `skill_sync.go`:
  - GET `/v1/skills/search?scope=<scope>&agent_type=<agent>&status=published`
  - Compare against local `.claude/commands/` directory
  - Install missing/outdated; print `{installed, updated, orphaned}`

---

## Testing Tasks

All tests follow TDD: test file written before implementation file.

### Unit Tests

- [ ] `internal/config` — valid config loads; missing required fields return error; `"changeme"` token logs warning; env var overlay overrides YAML values
- [ ] `internal/auth/tokens.go` — `HashToken` is deterministic; `GenerateToken` produces `"pb_"` prefix; scope enforcement rejects out-of-scope requests; expired token rejected
- [ ] `internal/embedding/classifier.go` — Go source file classified as `"code"`; prose text classified as `"text"`; file with unknown extension falls back to content heuristic
- [ ] `internal/memory/recall.go` — combined score formula produces correct values for known inputs; `min_score` filter excludes low-scoring results; `strict_scope=true` returns only the target scope
- [ ] `internal/retrieval/merge.go` — promoted memory is deduplicated when knowledge artifact is also present; knowledge boost of +0.1 applied; results sorted DESC by score
- [ ] `internal/skills/invoke.go` — `$PARAM_NAME` substituted correctly; `{{param_name}}` substituted correctly; missing required param returns `ErrValidation`; wrong enum value returns `ErrValidation`; integer type validation rejects string value
- [ ] `internal/knowledge/lifecycle.go` — self-endorsement returns `ErrSelfEndorsement`; auto-publish fires when `endorsement_count >= review_required`; `Deprecate` rejects non-admin caller; `EmergencyRollback` clears `published_at` and `deprecated_at`
- [ ] `internal/principals/membership.go` — cycle detection rejects A→B→A; direct self-loop rejected by DB constraint; `IsScopeAdmin` returns true for ancestor-scope admin

### Integration Tests (require real PostgreSQL via testcontainers)

- [ ] `internal/db` — all 5 migrations apply cleanly in order; down migrations reverse cleanly; `CheckAndMigrate` acquires advisory lock and blocks concurrent call; version-ahead guard refuses to start
- [ ] Memory lifecycle — `Create` returns `action:"updated"` for near-duplicate (cosine ≤ 0.05); `Create` with `memory_type="working"` and no `expires_in` sets `expires_at = now()+3600s`; `SoftDelete` excludes from `Recall`; `HardDelete` removes row
- [ ] Scope fan-out — querying `project:acme/api` returns memories from project, team, department, and company scopes; querying with `max_scope_depth=1` returns only project scope; `strict_scope=true` returns only exact scope
- [ ] Knowledge promotion workflow — nomination creates pending request; approval transaction creates artifact, sets `promoted_to` and `promotion_status="promoted"` atomically; re-nomination of already-promoted memory is rejected
- [ ] Knowledge endorsement → auto-publish — artifact reaches `review_required` endorsements and transitions to `published`; self-endorsement rejected; non-admin cannot deprecate
- [ ] Staleness flags — `source_modified` flag inserted via Go; duplicate open flag not inserted; `low_access_age` pg_cron job fires (use `cron.schedule` with 1-minute interval in test) and inserts flag for qualifying artifact
- [ ] Skill install/invoke/search — `Install` writes correct frontmatter + body to `.claude/commands/<slug>.md`; `Invoke` with valid params returns expanded body; `Invoke` with missing required param returns 422; `Recall` returns skill when query matches description

### E2E Tests

- [ ] MCP tool calls via mcp-go test client — `remember` → `recall` → `forget` round-trip; `publish` → `endorse` → auto-publish flow; `skill_search` returns installed flag correctly
- [ ] REST API via `net/http/httptest` — all CRUD endpoints return correct status codes; pagination `next_cursor` advances correctly; unauthenticated request returns 401; out-of-scope token returns 403

---

## Observability Tasks

- [ ] Prometheus metrics (expose on `/metrics`):
  - `postbrain_tool_duration_seconds{tool}` histogram — p50/p99 per MCP tool
  - `postbrain_embedding_duration_seconds{backend,model}` histogram
  - `postbrain_job_duration_seconds{job}` histogram
  - `postbrain_active_memories_total{scope}` gauge (updated on write/delete)
  - `postbrain_recall_results_total{layer}` counter
- [ ] `log/slog` structured logging — every log line includes: `request_id`, `principal_id`, `scope_id` where applicable; use `slog.With` at middleware level to inject fields
- [ ] `/health` endpoint: `{"status":"ok","schema_version":N,"expected_version":M,"schema_dirty":false}` — returns 503 if dirty or version mismatch
