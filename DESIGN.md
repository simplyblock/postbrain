# Postbrain — Long-Term Memory for Coding Agents

## Overview

Postbrain is a persistent, queryable memory backend for AI coding agents (Claude Code, OpenAI Codex, GitHub Copilot, and similar tools) and the human developers who work alongside them. It provides three complementary stores:

1. **Memory** — automatically captured observations from agent sessions and developer interactions. Personal by default, scoped to a project or team, subject to decay and consolidation.
2. **Knowledge** — intentionally curated artifacts that are reviewed, versioned, and explicitly published to a chosen audience: a project, a team, a department, or the whole company.
3. **Skills** — versioned, parameterised prompt templates that agents can discover, install, and invoke. A centralised registry for reusable agent behaviours — the `/deploy`, `/review-pr`, and `/write-tests` commands that live in `.claude/commands/` today, stored and shared through Postbrain instead of scattered across machines and repos.

All three layers are queryable together in a single call. The separations are not cosmetic — each reflects a real difference in provenance, trust, lifecycle, executability, and sharing semantics.

The system is built on PostgreSQL with the `pg_vector` extension, chosen for its ability to store both structured relational data (facts, relationships, metadata) and dense vector embeddings (semantic similarity search) in a single, ACID-compliant database.

---

## Goals

- **Persistence** — memories and knowledge survive session termination, context window resets, and agent upgrades.
- **Semantic recall** — agents and developers retrieve content by meaning, not just exact keyword match.
- **Multi-principal** — agents, individual developers, teams, departments, and whole companies are all first-class actors.
- **Intentional sharing** — knowledge and skills are explicitly published at a chosen visibility level; never accidentally leaked by scope inheritance.
- **Promotion pathway** — any memory can be nominated and reviewed for elevation into a persistent, shared knowledge artifact; any procedural knowledge artifact can be promoted into an executable skill.
- **Skill registry** — a centralised store for versioned, parameterised agent skills, discoverable by meaning and auto-installable into agent command directories.
- **Auditability** — every write is attributed, timestamped, and versioned.
- **Low latency** — p99 retrieval under 50 ms for typical workloads.
- **Composability** — language-agnostic HTTP/JSON API and MCP server; no language-specific SDK required.

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────────────┐
│                            Principal Layer                                │
│                                                                            │
│  ┌──────────────┐  ┌──────────────┐  ┌─────────────┐  ┌──────────────┐ │
│  │  Claude Code │  │  OpenAI      │  │  Developer  │  │  CI Pipeline │ │
│  │  (MCP hook)  │  │  Codex CLI   │  │  (Web UI /  │  │  (REST)      │ │
│  │              │  │              │  │   CLI)      │  │              │ │
│  └──────┬───────┘  └──────┬───────┘  └──────┬──────┘  └──────┬───────┘ │
│         └─────────────────┴─────────────────┴─────────────────┘         │
│                                    │                                       │
│                            MCP / REST / SDK                                │
└────────────────────────────────────┼─────────────────────────────────────┘
                                     │
┌────────────────────────────────────▼─────────────────────────────────────┐
│                            Postbrain Server                               │
│                                                                            │
│  ┌──────────────────┐  ┌─────────────────┐  ┌──────────────────────────┐│
│  │   MCP Server     │  │   REST API      │  │  Background Jobs         ││
│  │  (primary)       │  │  (fallback/ext) │  │  · consolidation         ││
│  │                  │  │                 │  │  · decay scoring         ││
│  │  Memory tools:   │  │  /v1/memories   │  │  · embedding sync        ││
│  │  · remember      │  │  /v1/knowledge  │  │  · TTL expiry            ││
│  │  · recall        │  │  /v1/context    │  │  · promotion review      ││
│  │  · forget        │  │  /v1/graph      │  └──────────────────────────┘│
│  │  · context       │  │  /v1/orgs       │                               │
│  │  · summarize     │  └─────────────────┘                               │
│  │                  │                                                      │
│  │  Knowledge tools:│  ┌─────────────────┐                               │
│  │  · publish       │  │  Embedding      │                               │
│  │  · endorse       │  │  Service        │                               │
│  │  · promote       │  │  (local/ext)    │                               │
│  │  · collect       │  └─────────────────┘                               │
│  │                  │                                                      │
│  │  Skill tools:    │                                                      │
│  │  · skill_search  │                                                      │
│  │  · skill_install │                                                      │
│  │  · skill_invoke  │                                                      │
│  └──────────────────┘                                                     │
└────────────────────────────────────┬─────────────────────────────────────┘
                                     │
┌────────────────────────────────────▼─────────────────────────────────────┐
│                         PostgreSQL + pg_vector                            │
│                                                                            │
│  principals  │  scopes  │  memories  │  knowledge_artifacts  │  skills    │
│  entities    │  relations            │  collections                        │
│  sharing_grants  │  staleness_flags  │  sessions  │  events                │
└───────────────────────────────────────────────────────────────────────────┘
```

### Components

| Component | Role |
|-----------|------|
| **MCP Server** | Primary integration point. Exposes Postbrain as a set of MCP tools that agents call via the Model Context Protocol. Claude Code connects here natively. |
| **REST API** | HTTP/JSON fallback for agents that don't speak MCP (scripts, Codex CLI wrappers, CI pipelines, developer web UIs). |
| **Embedding Service** | Converts text to dense vectors. Can be a local model (e.g., `nomic-embed-text` via Ollama) or a remote API (OpenAI `text-embedding-3-small`). Swappable via config. |
| **Background Jobs** | Periodic consolidation (merge near-duplicate memories), decay scoring, embedding re-sync when the model changes, pruning of low-relevance ephemeral memories, promotion review notifications. |
| **PostgreSQL + pg_vector** | Single source of truth. All data lives here. |

---

## Three-Layer Model: Memory, Knowledge, and Skills

### Design Decision

The three stores are separated into distinct tables. Each differs across every dimension that matters for implementation:

| Dimension | Memory | Knowledge | Skill |
|-----------|--------|-----------|-------|
| **Authored by** | Agent or developer, automatically | Human developer, intentionally | Human developer, intentionally |
| **Default visibility** | Private to owner/scope | Chosen explicitly at publish time | Chosen explicitly at publish time |
| **Trust level** | Observed, potentially noisy | Reviewed, institutionally endorsed | Reviewed, tested, institutionally endorsed |
| **Lifecycle** | Written → decays → consolidated/pruned | Draft → Review → Published → Deprecated | Draft → Review → Published → Deprecated |
| **Decay** | Yes — importance degrades over time | No | No |
| **Write frequency** | Very high (100s/session) | Low (deliberate authoring) | Very low (intentional publishing) |
| **Versioning** | Implicit (new write supersedes) | Explicit (full version history) | Explicit (full version history) |
| **Sharing** | Via scope hierarchy or grant | Via explicit visibility level | Via explicit visibility level |
| **Executable** | No | No | Yes — invoked by agents with typed parameters |
| **Agent-specific** | No | No | Yes — compatibility declared per agent type |
| **Installation** | N/A | N/A | Materialised as a file (e.g. `.claude/commands/*.md`) |
| **Telemetry** | Access count | Access count | Invocation count, last invoked |

Combining them into one table would pollute every query with executability and compatibility discriminators and make the lifecycle machinery unmanageably complex.

The three layers are linked by **promotion pathways**:
- Any memory can be nominated for elevation into a knowledge artifact.
- Any `procedural` knowledge artifact can be promoted into an executable skill (parameterised and made agent-compatible).

---

## Memory Taxonomy

Postbrain models memory in four orthogonal types, mirroring how human/agent cognition actually works:

| Type | Description | Examples |
|------|-------------|---------|
| **Semantic** | General facts about the world, codebase, or domain. No specific time. | "This repo uses Hexagonal Architecture", "The auth service owns JWTs", "noctarius prefers short responses" |
| **Episodic** | Specific events or interactions, time-stamped. | "On 2026-03-20, agent fixed the N+1 query in UserRepository", "User rejected approach X for reason Y" |
| **Procedural** | How-to knowledge: workflows, patterns, runbooks. | "To release: run `make tag`, then push to release branch", "Always run `go vet` before committing" |
| **Working** | Short-lived context for the current task. Auto-expires after a TTL. | "Currently refactoring the payment module", "PR #42 is in review" |

## Knowledge Taxonomy

Knowledge artifacts use the same four types but carry additional classification:

| Type | Description | Examples |
|------|-------------|---------|
| **Semantic** | Agreed facts: architectural decisions, system boundaries, domain definitions. | "All PII must pass through the data-classification service before storage" |
| **Episodic** | Significant recorded events: post-mortems, architectural decisions (ADRs). | "2026-01-14 — migrated auth from session cookies to JWTs; see ADR-023" |
| **Procedural** | Official runbooks, coding standards, release processes. | "Security incident response playbook", "Frontend accessibility checklist" |
| **Reference** | Stable pointers: API contracts, schema definitions, external docs. | "Stripe webhook event catalog", "internal service mesh port assignments" |

Knowledge artifacts belong to **collections** (curated bundles) and carry an explicit **visibility level** that determines who can read them.

## Skills Taxonomy

Skills are reusable, parameterised prompt templates that agents can invoke. They are the executable counterpart to procedural knowledge artifacts.

| Attribute | Description |
|-----------|-------------|
| **slug** | The command name: `deploy`, `review-pr`, `write-tests` |
| **body** | The prompt template, written in the agent's native format (Claude Code markdown, Codex system prompt fragment, etc.) |
| **parameters** | Typed parameter schema: `[{name, type, required, default, description}]`. Types: `string`, `integer`, `boolean`, `enum`. |
| **agent_types** | Which agents can execute this skill: `["claude-code"]`, `["codex"]`, `["any"]` |
| **visibility** | Same levels as knowledge artifacts: `private`, `project`, `team`, `department`, `company` |

**Parameter substitution** follows the same convention Claude Code uses today — `$PARAM_NAME` or `{{param_name}}` in the body is replaced at invocation time. The skill body is otherwise a standard agent prompt.

**Example skill — `review-pr`:**
```markdown
---
name: Review Pull Request
description: Review a pull request for correctness, security, and style
agent_types: [claude-code]
parameters:
  - name: pr_number
    type: integer
    required: true
  - name: focus
    type: enum
    values: [security, performance, style, all]
    default: all
---

Review PR #$pr_number with focus on $focus issues.

Use the `gh pr diff $pr_number` command to fetch the diff, then:
1. Identify any $focus problems with specific file:line references
2. Suggest concrete fixes, not just observations
3. Note anything that looks like a security concern regardless of focus
```

This skill lives in Postbrain rather than in a local `.claude/commands/` file. Any developer in the owning scope can install it with one command; updates propagate automatically.

---

## Principal Model

All actors in the system — agents, human developers, teams, departments, and companies — are modelled as **principals**. This replaces the earlier `agents` table and provides a unified identity layer.

### Principal types

| Kind | Description | Examples |
|------|-------------|---------|
| `agent` | An AI coding agent instance | `claude-code`, `codex`, `copilot` |
| `user` | A human developer | `alice@acme.com` |
| `team` | A group of users and agents | `platform-engineering`, `mobile` |
| `department` | A business unit containing teams | `engineering`, `product`, `security` |
| `company` | The top-level organization | `acme-inc` |

### Membership

Principals form a hierarchy via `principal_memberships`:
- A `user` can belong to one or more `team`s.
- A `team` belongs to one `department`.
- A `department` belongs to one `company`.
- An `agent` is associated with a `user` (the person who connected it) or a `team` (when shared, e.g., a CI agent).

The membership graph determines scope access: a user's effective read scopes are the union of their personal scope, all their teams' scopes, their department's scope, and the company scope.

---

## Scope Model

Scopes form a five-level hierarchy. The scope hierarchy governs **memory reads** — a query fans up the tree automatically. Knowledge visibility is governed separately (see below).

```
company:acme
  ├── department:engineering
  │     ├── team:platform
  │     │     ├── project:acme/api
  │     │     └── project:acme/cli
  │     └── team:mobile
  │           └── project:acme/ios
  └── department:product
        └── team:design
              └── project:acme/design-system

user:alice    (personal — not in the main hierarchy)
user:bob
```

### Memory read fan-out

When an agent queries scope `project:acme/api`:
1. Results from `project:acme/api` (highest specificity, highest weight)
2. Results from `team:platform`
3. Results from `department:engineering`
4. Results from `company:acme`
5. Results from `user:<current-user>` (personal memories, always included)

The fan-out is configurable: agents can pass `max_scope_depth: 2` to limit the walk to project + team, or `strict_scope: true` to disable fan-out entirely.

### Memory write target

Writes always go to exactly one scope, specified by the caller. An agent writing in a project writes to `project:acme/api`. A developer tagging something as a team convention writes to `team:platform`. Writes can never silently target a broader scope than the token allows.

---

## Database Design

### Schema Overview

```sql
-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS vector;        -- ANN / embedding search (pg_vector)
CREATE EXTENSION IF NOT EXISTS pg_trgm;       -- trigram fuzzy/partial keyword search
CREATE EXTENSION IF NOT EXISTS btree_gin;     -- GIN indexes on btree-indexable types
CREATE EXTENSION IF NOT EXISTS ltree;         -- scope hierarchy paths and operators
CREATE EXTENSION IF NOT EXISTS citext;        -- case-insensitive text for slugs and emails
CREATE EXTENSION IF NOT EXISTS unaccent;      -- strip accents from FTS tokens (international teams)
CREATE EXTENSION IF NOT EXISTS fuzzystrmatch; -- Levenshtein / Soundex for entity deduplication
CREATE EXTENSION IF NOT EXISTS pg_prewarm;    -- warm HNSW indexes into shared_buffers on startup
CREATE EXTENSION IF NOT EXISTS pg_cron;       -- in-database scheduling for housekeeping jobs
CREATE EXTENSION IF NOT EXISTS pg_partman;    -- automated time-range partition management

-- ─────────────────────────────────────────
-- Custom FTS configuration with unaccent
-- (used on all content / title columns)
-- ─────────────────────────────────────────
CREATE TEXT SEARCH CONFIGURATION postbrain_fts (COPY = pg_catalog.english);
ALTER TEXT SEARCH CONFIGURATION postbrain_fts
    ALTER MAPPING FOR hword, hword_part, word
    WITH unaccent, english_stem;

-- ─────────────────────────────────────────
-- Embedding model registry
-- ─────────────────────────────────────────
--
-- content_type distinguishes text models (marketing copy, decisions,
-- natural language memories) from code models (source fragments, diffs,
-- API signatures). A deployment configures exactly one active default
-- per content_type; changing it triggers the reembed background job.

CREATE TABLE embedding_models (
    id               UUID PRIMARY KEY DEFAULT uuidv7(),
    slug             citext NOT NULL UNIQUE,  -- "openai/text-embedding-3-small", "voyage/voyage-code-3"
    dimensions       INT NOT NULL,            -- 768 | 1024 | 1536 | 3072
    content_type     TEXT NOT NULL DEFAULT 'text' CHECK (content_type IN ('text', 'code')),
    is_active        BOOLEAN NOT NULL DEFAULT false,  -- current default for this content_type
    description      TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Enforce at most one active model per content_type
CREATE UNIQUE INDEX embedding_models_active_text_idx
    ON embedding_models (is_active) WHERE is_active = true AND content_type = 'text';
CREATE UNIQUE INDEX embedding_models_active_code_idx
    ON embedding_models (is_active) WHERE is_active = true AND content_type = 'code';

-- ─────────────────────────────────────────
-- Principals (agents, users, teams,
--             departments, companies)
-- ─────────────────────────────────────────

CREATE TABLE principals (
    id           UUID PRIMARY KEY DEFAULT uuidv7(),
    kind         TEXT NOT NULL CHECK (kind IN ('agent', 'user', 'team', 'department', 'company')),
    -- citext: "Alice@Acme.com" and "alice@acme.com" resolve to the same principal
    slug         citext NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    meta         JSONB NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Hierarchical membership: user → team → department → company
-- Also: agent → user (owner) or agent → team (shared agent)
CREATE TABLE principal_memberships (
    member_id   UUID NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
    parent_id   UUID NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
    role        TEXT NOT NULL DEFAULT 'member',  -- "member", "owner", "admin"
    granted_by  UUID REFERENCES principals(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (member_id, parent_id),
    CHECK (member_id <> parent_id)
);

CREATE INDEX principal_memberships_parent_idx ON principal_memberships (parent_id);

-- ─────────────────────────────────────────
-- Scopes (namespaces for memory & knowledge)
-- ─────────────────────────────────────────

CREATE TABLE scopes (
    id           UUID PRIMARY KEY DEFAULT uuidv7(),
    kind         TEXT NOT NULL CHECK (kind IN ('user', 'project', 'team', 'department', 'company')),
    -- citext: scope lookups are case-insensitive ("Acme/API" == "acme/api")
    external_id  citext NOT NULL,
    name         TEXT NOT NULL,
    parent_id    UUID REFERENCES scopes(id),  -- NULL for company and user (roots)
    principal_id UUID NOT NULL REFERENCES principals(id),
    -- ltree path: computed automatically by trigger below.
    -- Labels are the external_id with non-alphanumeric chars replaced by '_'.
    -- Examples:
    --   company:acme             → acme
    --   department:engineering   → acme.engineering
    --   team:platform            → acme.engineering.platform
    --   project:acme/api         → acme.engineering.platform.acme_api
    --   user:alice@acme.com      → user.alice_at_acme_com  (separate root)
    path         ltree NOT NULL,
    meta         JSONB NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (kind, external_id)
);

-- GiST index on path enables O(log n) ancestor (@>) and descendant (<@) queries
-- without recursive CTEs.
CREATE INDEX scopes_path_gist_idx ON scopes USING gist (path);
CREATE INDEX scopes_path_btree_idx ON scopes USING btree (path);
CREATE INDEX scopes_parent_idx     ON scopes (parent_id) WHERE parent_id IS NOT NULL;

-- Trigger: auto-compute path on insert/update from parent's path + sanitized label
CREATE OR REPLACE FUNCTION scopes_compute_path()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE
    parent_path ltree;
    safe_label  TEXT;
BEGIN
    -- Sanitize: replace any char outside [a-zA-Z0-9_] with '_'
    safe_label := regexp_replace(NEW.external_id, '[^a-zA-Z0-9_]', '_', 'g');

    IF NEW.parent_id IS NULL THEN
        -- Root node: company scope or personal user scope
        NEW.path := CASE NEW.kind
            WHEN 'user'    THEN text2ltree('user.' || safe_label)
            ELSE                 text2ltree(safe_label)
        END;
    ELSE
        SELECT path INTO parent_path FROM scopes WHERE id = NEW.parent_id;
        NEW.path := parent_path || text2ltree(safe_label);
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER scopes_path_trigger
    BEFORE INSERT OR UPDATE OF parent_id, external_id ON scopes
    FOR EACH ROW EXECUTE FUNCTION scopes_compute_path();

-- ─────────────────────────────────────────
-- Memory store (primary table)
-- ─────────────────────────────────────────

CREATE TABLE memories (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),

    -- Classification
    memory_type     TEXT NOT NULL CHECK (memory_type IN ('semantic', 'episodic', 'procedural', 'working')),
    scope_id        UUID NOT NULL REFERENCES scopes(id) ON DELETE CASCADE,
    author_id       UUID NOT NULL REFERENCES principals(id),  -- the agent or user who wrote this

    -- Content
    content              TEXT NOT NULL,
    summary              TEXT,
    -- Text embedding: populated for all memories; dimension = active text model
    embedding            vector(1536),
    embedding_model_id   UUID REFERENCES embedding_models(id),
    -- Code embedding: populated when content_kind = 'code'; dimension = active code model.
    -- NULL for natural-language memories (marketing copy, decisions, conversations).
    embedding_code       vector(1024),
    embedding_code_model_id UUID REFERENCES embedding_models(id),
    -- Whether this memory's content is primarily code or natural language.
    -- Set by the embedding service at write time via heuristic + source_ref inspection.
    content_kind         TEXT NOT NULL DEFAULT 'text' CHECK (content_kind IN ('text', 'code')),
    meta                 JSONB NOT NULL DEFAULT '{}',

    -- Versioning & lifecycle
    version         INT NOT NULL DEFAULT 1,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    confidence      FLOAT NOT NULL DEFAULT 1.0 CHECK (confidence BETWEEN 0 AND 1),
    importance      FLOAT NOT NULL DEFAULT 0.5 CHECK (importance BETWEEN 0 AND 1),
    access_count    INT NOT NULL DEFAULT 0,
    last_accessed   TIMESTAMPTZ,

    -- TTL for working memory
    expires_at      TIMESTAMPTZ,

    -- Promotion tracking
    promotion_status  TEXT CHECK (promotion_status IN ('none', 'nominated', 'promoted'))
                      NOT NULL DEFAULT 'none',
    promoted_to       UUID,   -- FK set after insert; references knowledge_artifacts(id)

    -- Provenance
    source_ref      TEXT,   -- e.g. "file:src/auth/jwt.go:42", "pr:123", "conversation:abc"
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Text embedding: primary ANN index for natural-language recall
CREATE INDEX memories_embedding_hnsw_idx
    ON memories USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- Code embedding: ANN index used when search_mode = 'code' or content_kind = 'code'
-- Partial index keeps it small — only code memories are indexed here.
CREATE INDEX memories_embedding_code_hnsw_idx
    ON memories USING hnsw (embedding_code vector_cosine_ops)
    WITH (m = 16, ef_construction = 64)
    WHERE embedding_code IS NOT NULL;

-- Full-text search (fallback and hybrid re-ranking)
CREATE INDEX memories_content_fts_idx
    ON memories USING GIN (to_tsvector('postbrain_fts', content));

-- Trigram index for partial-match / fuzzy keyword search
CREATE INDEX memories_content_trgm_idx
    ON memories USING GIN (content gin_trgm_ops);

-- Composite index for scope-filtered ANN queries
CREATE INDEX memories_scope_type_idx
    ON memories (scope_id, memory_type, is_active);

-- Auto-expire working memory
CREATE INDEX memories_expires_at_idx
    ON memories (expires_at)
    WHERE expires_at IS NOT NULL;

-- ─────────────────────────────────────────
-- Entity registry (named things)
-- ─────────────────────────────────────────

CREATE TABLE entities (
    id                 UUID PRIMARY KEY DEFAULT uuidv7(),
    scope_id           UUID NOT NULL REFERENCES scopes(id) ON DELETE CASCADE,
    entity_type        TEXT NOT NULL,   -- "file", "function", "concept", "person", "service", "pr", …
    name               TEXT NOT NULL,
    canonical          TEXT NOT NULL,   -- normalized identifier (file path, FQN, slug)
    meta               JSONB NOT NULL DEFAULT '{}',
    embedding          vector(1536),
    embedding_model_id UUID REFERENCES embedding_models(id),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (scope_id, entity_type, canonical)
);

CREATE INDEX entities_embedding_hnsw_idx
    ON entities USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- ─────────────────────────────────────────
-- Memory ↔ Entity links
-- ─────────────────────────────────────────

CREATE TABLE memory_entities (
    memory_id   UUID NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    entity_id   UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    role        TEXT,   -- "subject", "object", "context", …
    PRIMARY KEY (memory_id, entity_id)
);

-- ─────────────────────────────────────────
-- Entity ↔ Entity relations (knowledge graph)
-- ─────────────────────────────────────────

CREATE TABLE relations (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    scope_id        UUID NOT NULL REFERENCES scopes(id) ON DELETE CASCADE,
    subject_id      UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    predicate       TEXT NOT NULL,   -- "owns", "depends_on", "implements", "authored_by", …
    object_id       UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    confidence      FLOAT NOT NULL DEFAULT 1.0,
    source_memory   UUID REFERENCES memories(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (scope_id, subject_id, predicate, object_id)
);

CREATE INDEX relations_subject_idx ON relations (subject_id, predicate);
CREATE INDEX relations_object_idx  ON relations (object_id, predicate);

-- ─────────────────────────────────────────
-- Conversation / session log
-- ─────────────────────────────────────────

CREATE TABLE sessions (
    id           UUID PRIMARY KEY DEFAULT uuidv7(),
    scope_id     UUID NOT NULL REFERENCES scopes(id) ON DELETE CASCADE,
    principal_id UUID REFERENCES principals(id),  -- the agent or user running this session
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at     TIMESTAMPTZ,
    meta         JSONB NOT NULL DEFAULT '{}'
);

-- events is an append-only audit log; it grows unboundedly.
-- Partitioned by month so old partitions can be detached/archived cheaply.
-- pg_partman manages creation of future partitions and optional retention drops.
--
-- NOTE: PK must include the partition key (created_at) per PostgreSQL rules.
CREATE TABLE events (
    id          UUID        NOT NULL DEFAULT uuidv7(),
    session_id  UUID        NOT NULL,   -- no FK: FK across partitions is impractical
    scope_id    UUID        NOT NULL REFERENCES scopes(id) ON DELETE CASCADE,
    event_type  TEXT        NOT NULL,   -- "tool_call", "memory_write", "memory_read", …
    payload     JSONB       NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE INDEX events_session_idx ON events (session_id, created_at DESC);
CREATE INDEX events_scope_idx   ON events (scope_id, event_type, created_at DESC);

-- Hand partition management to pg_partman (run after table creation):
SELECT partman.create_parent(
    p_parent_table => 'public.events',
    p_control      => 'created_at',
    p_interval     => 'monthly',
    p_premake      => 3     -- pre-create 3 upcoming monthly partitions
);

-- Retention: detach partitions older than 24 months (data archived, not deleted)
UPDATE partman.part_config
    SET retention            = '24 months',
        retention_keep_table = true   -- detach, don't DROP
    WHERE parent_table = 'public.events';

-- ─────────────────────────────────────────
-- Knowledge layer
-- ─────────────────────────────────────────
--
-- Visibility levels (explicit, not inherited):
--   private     → only the owning scope can read
--   project     → all members of the project scope
--   team        → all members of the owning team
--   department  → all members of the owning department
--   company     → everyone in the company
--
-- Lifecycle: draft → in_review → published → deprecated

CREATE TABLE knowledge_artifacts (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),

    -- Classification
    knowledge_type  TEXT NOT NULL CHECK (knowledge_type IN ('semantic', 'episodic', 'procedural', 'reference')),
    owner_scope_id  UUID NOT NULL REFERENCES scopes(id) ON DELETE CASCADE,
    author_id       UUID NOT NULL REFERENCES principals(id),

    -- Sharing
    visibility      TEXT NOT NULL DEFAULT 'team'
                    CHECK (visibility IN ('private', 'project', 'team', 'department', 'company')),

    -- Lifecycle
    status          TEXT NOT NULL DEFAULT 'draft'
                    CHECK (status IN ('draft', 'in_review', 'published', 'deprecated')),
    published_at    TIMESTAMPTZ,
    deprecated_at   TIMESTAMPTZ,
    review_required INT NOT NULL DEFAULT 1,  -- min endorsements needed to auto-publish from in_review

    -- Content
    title                TEXT NOT NULL,
    content              TEXT NOT NULL,
    summary              TEXT,
    embedding            vector(1536),
    embedding_model_id   UUID REFERENCES embedding_models(id),
    meta                 JSONB NOT NULL DEFAULT '{}',

    -- Authority scoring
    endorsement_count INT NOT NULL DEFAULT 0,  -- denormalized for fast scoring
    access_count      INT NOT NULL DEFAULT 0,
    last_accessed     TIMESTAMPTZ,             -- updated on every read; used by staleness signal 3

    -- Version control
    version         INT NOT NULL DEFAULT 1,
    previous_version UUID REFERENCES knowledge_artifacts(id),  -- linked list of versions

    -- Provenance
    source_memory_id UUID REFERENCES memories(id) ON DELETE SET NULL,  -- if promoted from memory
    source_ref       TEXT,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX knowledge_embedding_hnsw_idx
    ON knowledge_artifacts USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

CREATE INDEX knowledge_owner_scope_idx
    ON knowledge_artifacts (owner_scope_id, visibility, status);

CREATE INDEX knowledge_content_fts_idx
    ON knowledge_artifacts USING GIN (to_tsvector('postbrain_fts', content));

CREATE INDEX knowledge_content_trgm_idx
    ON knowledge_artifacts USING GIN (content gin_trgm_ops);

-- Who has endorsed this knowledge artifact
CREATE TABLE knowledge_endorsements (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    artifact_id     UUID NOT NULL REFERENCES knowledge_artifacts(id) ON DELETE CASCADE,
    endorser_id     UUID NOT NULL REFERENCES principals(id),
    note            TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (artifact_id, endorser_id)
);

-- Version history (full content snapshots on each publish)
CREATE TABLE knowledge_history (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    artifact_id     UUID NOT NULL REFERENCES knowledge_artifacts(id) ON DELETE CASCADE,
    version         INT NOT NULL,
    content         TEXT NOT NULL,
    summary         TEXT,
    changed_by      UUID NOT NULL REFERENCES principals(id),
    change_note     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (artifact_id, version)
);

-- ─────────────────────────────────────────
-- Knowledge collections
-- ─────────────────────────────────────────
--
-- A collection is a curated, named bundle of knowledge artifacts.
-- Examples: "Engineering Standards", "Payments Architecture",
--           "Security Policies", "React Component Conventions"

CREATE TABLE knowledge_collections (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    scope_id    UUID NOT NULL REFERENCES scopes(id) ON DELETE CASCADE,
    owner_id    UUID NOT NULL REFERENCES principals(id),
    slug        citext NOT NULL,
    name        TEXT NOT NULL,
    description TEXT,
    visibility  TEXT NOT NULL DEFAULT 'team'
                CHECK (visibility IN ('private', 'project', 'team', 'department', 'company')),
    meta        JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (scope_id, slug)
);

CREATE TABLE knowledge_collection_items (
    collection_id   UUID NOT NULL REFERENCES knowledge_collections(id) ON DELETE CASCADE,
    artifact_id     UUID NOT NULL REFERENCES knowledge_artifacts(id) ON DELETE CASCADE,
    position        INT NOT NULL DEFAULT 0,   -- ordering within collection
    added_by        UUID NOT NULL REFERENCES principals(id),
    added_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (collection_id, artifact_id)
);

-- ─────────────────────────────────────────
-- Sharing grants (fine-grained sharing)
-- ─────────────────────────────────────────
--
-- Allows a specific memory or knowledge artifact to be shared
-- with a target scope without moving it or changing its owner scope.
-- Example: user:alice shares a personal memory with team:platform.

CREATE TABLE sharing_grants (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    -- Exactly one of these is set:
    memory_id       UUID REFERENCES memories(id) ON DELETE CASCADE,
    artifact_id     UUID REFERENCES knowledge_artifacts(id) ON DELETE CASCADE,

    grantee_scope_id UUID NOT NULL REFERENCES scopes(id) ON DELETE CASCADE,
    granted_by      UUID NOT NULL REFERENCES principals(id),
    can_reshare     BOOLEAN NOT NULL DEFAULT FALSE,  -- grantee can grant further
    expires_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CHECK (
        (memory_id IS NOT NULL AND artifact_id IS NULL) OR
        (memory_id IS NULL AND artifact_id IS NOT NULL)
    )
);

CREATE INDEX sharing_grants_grantee_idx ON sharing_grants (grantee_scope_id);
CREATE INDEX sharing_grants_memory_idx  ON sharing_grants (memory_id) WHERE memory_id IS NOT NULL;
CREATE INDEX sharing_grants_artifact_idx ON sharing_grants (artifact_id) WHERE artifact_id IS NOT NULL;

-- ─────────────────────────────────────────
-- Promotion queue (memory → knowledge)
-- ─────────────────────────────────────────

CREATE TABLE promotion_requests (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    memory_id       UUID NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    requested_by    UUID NOT NULL REFERENCES principals(id),
    target_scope_id UUID NOT NULL REFERENCES scopes(id),
    target_visibility TEXT NOT NULL
                    CHECK (target_visibility IN ('private', 'project', 'team', 'department', 'company')),
    proposed_title  TEXT,
    proposed_collection_id UUID REFERENCES knowledge_collections(id),
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'approved', 'rejected', 'merged')),
    reviewer_id     UUID REFERENCES principals(id),
    review_note     TEXT,
    reviewed_at     TIMESTAMPTZ,
    result_artifact_id UUID REFERENCES knowledge_artifacts(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX promotion_requests_memory_idx ON promotion_requests (memory_id);
CREATE INDEX promotion_requests_status_idx ON promotion_requests (status, target_scope_id);

-- ─────────────────────────────────────────
-- Skills registry
-- ─────────────────────────────────────────
--
-- Skills are versioned, parameterised prompt templates that agents
-- can discover, install, and invoke. They use the same visibility
-- and lifecycle model as knowledge_artifacts.
--
-- agent_types: {"claude-code","codex","any"} — controls which
--              agents the install command materialises this for.
--
-- parameters: JSON array of parameter descriptors:
--   [{name, type, required, default, description, values?}]
--   type ∈ {string, integer, boolean, enum}
--
-- body: the raw prompt template. Parameter substitution uses
--   $PARAM_NAME (Claude Code convention) or {{param_name}}.

CREATE TABLE skills (
    id                 UUID PRIMARY KEY DEFAULT uuidv7(),
    scope_id           UUID NOT NULL REFERENCES scopes(id) ON DELETE CASCADE,
    author_id          UUID NOT NULL REFERENCES principals(id),
    -- Optional: promoted from a procedural knowledge artifact
    source_artifact_id UUID REFERENCES knowledge_artifacts(id) ON DELETE SET NULL,

    -- Identity
    slug               citext NOT NULL,   -- "deploy", "review-pr", "write-tests"
    name               TEXT NOT NULL,
    description        TEXT NOT NULL,     -- used for embedding-based discovery

    -- Compatibility
    agent_types        TEXT[] NOT NULL DEFAULT '{"any"}',

    -- Content
    body               TEXT NOT NULL,     -- the prompt template
    parameters         JSONB NOT NULL DEFAULT '[]',

    -- Sharing + lifecycle (mirrors knowledge_artifacts)
    visibility         TEXT NOT NULL DEFAULT 'team'
                       CHECK (visibility IN ('private','project','team','department','company')),
    status             TEXT NOT NULL DEFAULT 'draft'
                       CHECK (status IN ('draft','in_review','published','deprecated')),
    published_at       TIMESTAMPTZ,
    deprecated_at      TIMESTAMPTZ,
    review_required    INT NOT NULL DEFAULT 1,

    -- Versioning
    version            INT NOT NULL DEFAULT 1,
    previous_version   UUID REFERENCES skills(id),

    -- Embeddings (on description + body; for discovery via recall)
    embedding          vector(1536),
    embedding_model_id UUID REFERENCES embedding_models(id),

    -- Telemetry (denormalised from events for fast query)
    invocation_count   INT NOT NULL DEFAULT 0,
    last_invoked_at    TIMESTAMPTZ,

    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (scope_id, slug)
);

CREATE INDEX skills_embedding_hnsw_idx
    ON skills USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

CREATE INDEX skills_scope_status_idx
    ON skills (scope_id, status, visibility);

CREATE INDEX skills_content_fts_idx
    ON skills USING GIN (to_tsvector('postbrain_fts', description || ' ' || body));

-- Trigger: denormalise invocation stats from the events table
-- Called by the Go server after recording a skill_invoked event.
CREATE OR REPLACE FUNCTION skills_update_invocation_stats()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.event_type = 'skill_invoked' THEN
        UPDATE skills
        SET invocation_count  = invocation_count + 1,
            last_invoked_at   = NEW.created_at
        WHERE id = (NEW.payload->>'skill_id')::uuid;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER events_skill_stats
    AFTER INSERT ON events
    FOR EACH ROW EXECUTE FUNCTION skills_update_invocation_stats();

-- ─────────────────────────────────────────
-- Skill endorsements (same model as knowledge)
-- ─────────────────────────────────────────

CREATE TABLE skill_endorsements (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    skill_id    UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    endorser_id UUID NOT NULL REFERENCES principals(id),
    note        TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (skill_id, endorser_id)
);

-- ─────────────────────────────────────────
-- Skill version history
-- ─────────────────────────────────────────

CREATE TABLE skill_history (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    skill_id    UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    version     INT NOT NULL,
    body        TEXT NOT NULL,
    parameters  JSONB NOT NULL,
    changed_by  UUID NOT NULL REFERENCES principals(id),
    change_note TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (skill_id, version)
);

-- ─────────────────────────────────────────
-- Knowledge staleness flags
-- ─────────────────────────────────────────
--
-- Three signals can raise a flag independently:
--   source_modified        — a source_ref file was edited by an agent (hook-triggered, Go)
--   contradiction_detected — a recent memory contradicts the artifact (weekly LLM job, Go)
--   low_access_age         — artifact is old and unaccessed (monthly pg_cron, pure SQL)
--
-- Flags do NOT automatically change the artifact's status or score.
-- They annotate recall/context responses and populate the review queue.
-- A reviewer dismisses (still valid) or resolves (deprecated or updated) each flag.

CREATE TABLE staleness_flags (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    artifact_id UUID NOT NULL REFERENCES knowledge_artifacts(id) ON DELETE CASCADE,
    signal      TEXT NOT NULL CHECK (signal IN (
                    'source_modified',
                    'contradiction_detected',
                    'low_access_age'
                )),
    confidence  FLOAT NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    -- signal-specific evidence:
    --   source_modified:        {files: [...], session_id: "...", principal_id: "..."}
    --   contradiction_detected: {memory_ids: [...], classifier_verdict: "CONTRADICTS",
    --                            classifier_reasoning: "..."}
    --   low_access_age:         {last_accessed: "...", days_since_access: 173,
    --                            artifact_age_days: 412}
    evidence    JSONB NOT NULL DEFAULT '{}',
    status      TEXT NOT NULL DEFAULT 'open'
                CHECK (status IN ('open', 'dismissed', 'resolved')),
    flagged_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_by UUID REFERENCES principals(id),
    reviewed_at TIMESTAMPTZ,
    review_note TEXT
);

CREATE INDEX staleness_flags_artifact_idx ON staleness_flags (artifact_id, status);
CREATE INDEX staleness_flags_open_idx     ON staleness_flags (confidence DESC, flagged_at DESC)
    WHERE status = 'open';

-- ─────────────────────────────────────────
-- Memory consolidation audit log
-- ─────────────────────────────────────────

CREATE TABLE consolidations (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    scope_id        UUID NOT NULL REFERENCES scopes(id) ON DELETE CASCADE,
    source_ids      UUID[] NOT NULL,   -- memories that were merged/replaced
    result_id       UUID REFERENCES memories(id),
    strategy        TEXT NOT NULL,     -- "merge", "supersede", "prune"
    reason          TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ─────────────────────────────────────────
-- Housekeeping: auto-update updated_at
-- ─────────────────────────────────────────

CREATE OR REPLACE FUNCTION touch_updated_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN NEW.updated_at = now(); RETURN NEW; END;
$$;

CREATE TRIGGER memories_updated_at BEFORE UPDATE ON memories
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

CREATE TRIGGER entities_updated_at BEFORE UPDATE ON entities
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

CREATE TRIGGER knowledge_artifacts_updated_at BEFORE UPDATE ON knowledge_artifacts
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

CREATE TRIGGER principals_updated_at BEFORE UPDATE ON principals
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

CREATE TRIGGER knowledge_collections_updated_at BEFORE UPDATE ON knowledge_collections
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

CREATE TRIGGER skills_updated_at BEFORE UPDATE ON skills
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- ─────────────────────────────────────────
-- Forward FK: memories.promoted_to
-- ─────────────────────────────────────────
-- Applied after knowledge_artifacts is created:
ALTER TABLE memories
    ADD CONSTRAINT memories_promoted_to_fk
    FOREIGN KEY (promoted_to) REFERENCES knowledge_artifacts(id) ON DELETE SET NULL;

-- ─────────────────────────────────────────
-- pg_cron: in-database housekeeping jobs
-- ─────────────────────────────────────────
-- These replace the in-process Go scheduler for operations that only need
-- SQL. The Go server still handles LLM-assisted consolidation and promotion
-- notifications (which require external API calls).

-- Every 5 min: expire TTL-based working memories
SELECT cron.schedule('expire-working-memory', '*/5 * * * *', $$
    UPDATE memories
    SET    is_active = false
    WHERE  expires_at < now()
    AND    is_active = true
$$);

-- Nightly at 03:00: decay importance scores
-- Decay rates: working=0.015/day, episodic=0.010/day, semantic/procedural=0.005/day
SELECT cron.schedule('decay-memory-importance', '0 3 * * *', $$
    UPDATE memories
    SET    importance = GREATEST(0.0,
               importance * exp(
                   -CASE memory_type
                       WHEN 'working'    THEN 0.015
                       WHEN 'episodic'   THEN 0.010
                       ELSE                   0.005
                    END
                   * GREATEST(0, EXTRACT(EPOCH FROM
                       (now() - COALESCE(last_accessed, created_at))
                     ) / 86400.0)
               )
           )
    WHERE  is_active = true
$$);

-- Weekly on Sunday at 04:00: soft-delete low-value decayed memories
SELECT cron.schedule('prune-low-value-memories', '0 4 * * 0', $$
    UPDATE memories
    SET    is_active = false
    WHERE  is_active = true
    AND    importance < 0.05
    AND    access_count < 2
    AND    memory_type IN ('episodic', 'working')
$$);

-- Hourly: pg_partman partition maintenance (creates upcoming partitions)
SELECT cron.schedule('partman-maintenance', '0 * * * *',
    'SELECT partman.run_maintenance_proc()'
);

-- Monthly on the 1st at 06:00: flag published artifacts that are old and unaccessed
-- Staleness signal 3 — pure SQL, no LLM required.
SELECT cron.schedule('detect-stale-knowledge-age', '0 6 1 * *', $$
    INSERT INTO staleness_flags (artifact_id, signal, confidence, evidence)
    SELECT
        ka.id,
        'low_access_age',
        0.3,
        jsonb_build_object(
            'last_accessed',     ka.last_accessed,
            'days_since_access', EXTRACT(EPOCH FROM (now() - COALESCE(ka.last_accessed, ka.created_at))) / 86400,
            'artifact_age_days', EXTRACT(EPOCH FROM (now() - ka.created_at)) / 86400
        )
    FROM knowledge_artifacts ka
    WHERE ka.status = 'published'
    AND   ka.created_at < now() - INTERVAL '180 days'
    AND   COALESCE(ka.last_accessed, ka.created_at) < now() - INTERVAL '60 days'
    AND   NOT EXISTS (
              SELECT 1 FROM staleness_flags sf
              WHERE  sf.artifact_id = ka.id
              AND    sf.signal       = 'low_access_age'
              AND    sf.status       = 'open'
          )
$$);

-- ─────────────────────────────────────────
-- pg_prewarm: warm HNSW indexes on startup
-- ─────────────────────────────────────────
-- Add to postgresql.conf:  shared_preload_libraries = 'pg_prewarm,pg_cron,pg_partman'
-- The block below can be run as a startup script or via pg_cron @reboot equivalent.
--
-- SELECT pg_prewarm('memories_embedding_hnsw_idx');
-- SELECT pg_prewarm('memories_embedding_code_hnsw_idx');
-- SELECT pg_prewarm('knowledge_embedding_hnsw_idx');
-- SELECT pg_prewarm('entities_embedding_hnsw_idx');
```

---

## API Design

### MCP Tools (primary integration)

Agents that support the Model Context Protocol connect to Postbrain as an MCP server. The following tools are exposed:

#### `remember`
Store a new memory or update an existing one.

```jsonc
// Input
{
  "content":      "The payment service owns all Stripe webhook processing",
  "memory_type":  "semantic",          // semantic | episodic | procedural | working
  "scope":        "project:acme/api",
  "importance":   0.8,                 // optional, 0–1
  "source_ref":   "file:src/payments/webhooks.go:1",  // optional
  "entities":     ["payment-service", "stripe"],       // optional named entities
  "expires_in":   null                 // optional seconds, for working memory
}

// Output
{
  "memory_id": "018e4f2a-...",
  "action":    "created"   // or "updated" if near-duplicate found
}
```

#### `recall`
Retrieve memories **and** published knowledge semantically relevant to a query. Results from both layers are ranked together; each result carries a `layer` field so agents can distinguish them.

```jsonc
// Input
{
  "query":        "how does stripe webhook processing work",
  "scope":        "project:acme/api",
  "memory_types": ["semantic", "procedural"],             // optional filter
  "layers":       ["memory", "knowledge", "skills"],      // default: all three
  "agent_type":   "claude-code",                          // filters skill compatibility
  "limit":        10,
  "min_score":    0.70
}

// Output
{
  "results": [
    {
      "layer":       "knowledge",
      "id":          "0194ab3c-...",
      "title":       "Payment Service Architecture",
      "content":     "The payment service owns all Stripe webhook processing ...",
      "score":       0.97,
      "memory_type": "semantic",
      "visibility":  "team",
      "status":      "published",
      "endorsements": 4,
      "collection":  "payments-architecture"
    },
    {
      "layer":       "memory",
      "id":          "018e4f2a-...",
      "content":     "Payment service uses idempotency keys for all Stripe calls",
      "score":       0.88,
      "memory_type": "semantic",
      "source_ref":  "file:src/payments/stripe.go:14",
      "author":      "claude-code",
      "created_at":  "2026-03-20T14:22:00Z"
    },
    {
      "layer":            "skill",
      "id":               "0197cc1a-...",
      "slug":             "test-webhook",
      "name":             "Test Stripe Webhook",
      "description":      "Sends a test Stripe webhook event and verifies the handler response",
      "score":            0.81,
      "visibility":       "team",
      "agent_types":      ["claude-code"],
      "invocation_count": 47,
      "installed":        false
    }
  ]
}
```

#### `forget`
Deactivate or permanently delete a memory.

```jsonc
// Input
{
  "memory_id": "018e4f2a-...",
  "hard":      false   // false = soft-delete (is_active=false), true = permanent
}
```

#### `context`
Retrieve a structured context bundle for a new session — a curated set of the most relevant memories and published knowledge for the current scope, ready to inject into a system prompt. Knowledge blocks are always included first (they carry the highest institutional authority).

```jsonc
// Input
{
  "scope":       "project:acme/api",
  "query":       "I'm about to work on the payments module",
  "max_tokens":  4000
}

// Output
{
  "context_blocks": [
    {
      "layer":   "knowledge",
      "type":    "procedural",
      "title":   "Release Process",
      "content": "...",
      "collection": "engineering-standards"
    },
    {
      "layer":   "knowledge",
      "type":    "semantic",
      "title":   "Payment Service Boundaries",
      "content": "..."
    },
    {
      "layer":   "memory",
      "type":    "episodic",
      "content": "On 2026-03-18, rewrote the webhook retry logic to use exponential backoff"
    }
  ],
  "total_tokens": 1842
}
```

#### `summarize`
Ask Postbrain to consolidate a set of memories into a higher-level semantic memory (calls the embedding service + an LLM summarizer).

```jsonc
// Input
{
  "scope":  "project:acme/api",
  "topic":  "payment architecture",
  "dry_run": false
}
```

#### `publish` _(knowledge tool)_
Create or update a knowledge artifact. Developers and agents with write access to the target scope can call this; the artifact starts as `draft` unless `auto_review: true` is passed.

```jsonc
// Input
{
  "title":         "Payment Service Architecture",
  "content":       "...",
  "knowledge_type": "semantic",
  "scope":         "team:platform",
  "visibility":    "department",       // private | project | team | department | company
  "collection":    "payments-architecture",
  "auto_review":   false               // set true to move directly to in_review
}

// Output
{
  "artifact_id": "0194ab3c-...",
  "status":      "draft",
  "version":     1
}
```

#### `endorse` _(knowledge tool)_
Endorse a knowledge artifact. When endorsement count reaches `review_required`, the artifact is automatically promoted to `published`.

```jsonc
// Input
{
  "artifact_id": "0194ab3c-...",
  "note":        "Verified against current codebase on 2026-03-22"
}
```

#### `promote` _(knowledge tool)_
Nominate a memory for elevation to a knowledge artifact. Creates a `promotion_request` that can be reviewed by appropriate principals.

```jsonc
// Input
{
  "memory_id":    "018e4f2a-...",
  "title":        "Stripe Idempotency Key Convention",
  "visibility":   "team",
  "collection":   "payments-architecture",
  "note":         "This pattern should be documented for all payment engineers"
}

// Output
{
  "promotion_id": "019c00aa-...",
  "status":       "pending",
  "review_required_from": ["team:platform"]
}
```

#### `collect` _(knowledge tool)_
Create or manage a knowledge collection.

```jsonc
// Input — create
{
  "action":      "create",
  "slug":        "security-policies",
  "name":        "Security Policies",
  "description": "Mandatory reading for all engineers",
  "scope":       "department:engineering",
  "visibility":  "company"
}

// Input — add artifact
{
  "action":      "add",
  "collection":  "security-policies",
  "artifact_id": "0194ab3c-..."
}
```

#### `skill_search` _(skill tool)_
Find skills relevant to the current task by semantic similarity. Called automatically at session start to populate the agent's available command set; also callable on demand.

```jsonc
// Input
{
  "query":       "deploy to production",
  "scope":       "project:acme/api",
  "agent_type":  "claude-code",    // filter to compatible skills only
  "limit":       5
}

// Output
{
  "skills": [
    {
      "id":          "0197cc1a-...",
      "slug":        "deploy",
      "name":        "Deploy to Production",
      "description": "Runs the full deploy pipeline with smoke tests",
      "score":       0.96,
      "visibility":  "team",
      "parameters":  [{"name": "env", "type": "enum", "values": ["staging","prod"], "default": "staging"}],
      "invocation_count": 214,
      "installed":   false    // whether already in the agent's command directory
    }
  ]
}
```

#### `skill_install` _(skill tool)_
Materialise a skill into the agent's local command directory. For Claude Code, writes a `.md` file to `.claude/commands/<slug>.md` (project-level) or `~/.claude/commands/<slug>.md` (user-level).

```jsonc
// Input
{
  "skill_id": "0197cc1a-...",
  "target":   "project"   // "project" (.claude/commands/) or "user" (~/.claude/commands/)
}

// Output
{
  "path":    ".claude/commands/deploy.md",
  "version": 3,
  "action":  "created"   // or "updated" if already installed at an older version
}
```

#### `skill_invoke` _(skill tool)_
Record a skill invocation event (telemetry). Called by the agent after executing a skill so Postbrain can track usage. Does not execute the skill — the agent does that locally after installation.

```jsonc
// Input
{
  "skill_id":   "0197cc1a-...",
  "parameters": {"env": "prod"},
  "scope":      "project:acme/api"
}
// Records a skill_invoked event; updates invocation_count + last_invoked_at.
```

---

### REST API

For agents or scripts that cannot use MCP:

**Memory endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/memories` | Create a memory |
| `GET` | `/v1/memories/recall` | Recall (`?q=...&scope=...&layers=memory,knowledge`) |
| `GET` | `/v1/memories/:id` | Fetch a single memory |
| `PATCH` | `/v1/memories/:id` | Update content or metadata |
| `DELETE` | `/v1/memories/:id` | Soft- or hard-delete |
| `POST` | `/v1/memories/:id/promote` | Nominate for promotion |
| `GET` | `/v1/context` | Context bundle (`?scope=...&q=...&max_tokens=...`) |

**Knowledge endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/knowledge` | Create knowledge artifact |
| `GET` | `/v1/knowledge/search` | Search (`?q=...&scope=...&visibility=...`) |
| `GET` | `/v1/knowledge/:id` | Fetch artifact |
| `PATCH` | `/v1/knowledge/:id` | Update (creates new version) |
| `POST` | `/v1/knowledge/:id/endorse` | Endorse |
| `POST` | `/v1/knowledge/:id/deprecate` | Deprecate |
| `GET` | `/v1/knowledge/:id/history` | Full version history |

**Collections:**

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/collections` | Create collection |
| `GET` | `/v1/collections` | List (`?scope=...`) |
| `GET` | `/v1/collections/:slug` | Fetch with artifacts |
| `POST` | `/v1/collections/:slug/items` | Add artifact |
| `DELETE` | `/v1/collections/:slug/items/:id` | Remove artifact |

**Org / Principal management:**

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/orgs` | Register a company |
| `POST` | `/v1/orgs/:id/departments` | Create department |
| `POST` | `/v1/departments/:id/teams` | Create team |
| `POST` | `/v1/teams/:id/members` | Add user or agent to team |
| `GET` | `/v1/principals/:id/memberships` | Resolve membership chain |

**Sharing:**

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/sharing/grants` | Share memory or artifact with a scope |
| `DELETE` | `/v1/sharing/grants/:id` | Revoke grant |
| `GET` | `/v1/sharing/grants` | List grants for a principal |

**Skills:**

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/skills` | Create a skill |
| `GET` | `/v1/skills/search` | Search by query (`?q=...&scope=...&agent_type=...`) |
| `GET` | `/v1/skills/:id` | Fetch skill with current body |
| `PATCH` | `/v1/skills/:id` | Update (creates new version) |
| `POST` | `/v1/skills/:id/endorse` | Endorse |
| `POST` | `/v1/skills/:id/deprecate` | Deprecate |
| `GET` | `/v1/skills/:id/history` | Full version history |
| `POST` | `/v1/skills/:id/install` | Materialise to agent command dir |
| `POST` | `/v1/skills/:id/invoke` | Record invocation telemetry |
| `GET` | `/v1/skills/:id/stats` | Invocation stats (count, last used, top users) |

**Promotion review:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/promotions` | List pending promotion requests |
| `POST` | `/v1/promotions/:id/approve` | Approve and create knowledge artifact or skill |
| `POST` | `/v1/promotions/:id/reject` | Reject with note |

**Other:**

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/sessions` | Start a session |
| `PATCH` | `/v1/sessions/:id` | End a session |
| `GET` | `/v1/entities` | List or search entities |
| `GET` | `/v1/graph` | Traverse entity relation graph |

All endpoints accept and return `application/json`. Authentication is via a Bearer token issued per principal.

---

## Retrieval Strategy

Postbrain uses **hybrid retrieval** across both layers, merging results into a single ranked list.

### Step 1 — Candidate gathering (parallel)

Up to three sub-queries run in parallel depending on the `layers` param (`memory`, `knowledge`, `skills`; default: all three):

**Memory candidates** — ANN search against `memories.embedding` (HNSW), restricted to scopes reachable from the requested scope (fan-out walk) plus any scopes where the caller has a `sharing_grant`. Returns up to `limit × 3` candidates.

**Knowledge candidates** — ANN search against `knowledge_artifacts.embedding` (HNSW), restricted to published artifacts whose visibility level is reachable by the caller's principal membership chain. Returns up to `limit × 3` candidates.

**Skill candidates** — ANN search against `skills.embedding` (HNSW), restricted to published skills compatible with the calling agent type and reachable by visibility. Returns up to `limit × 2` candidates (skills are fewer in number than memories or knowledge).

All passes also run a parallel BM25 full-text query (`to_tsquery`) for keyword boost.

### Step 2 — Per-layer scoring

**Memory score:**
```
memory_score = (0.70 × cosine_similarity)
             + (0.15 × bm25_score)
             + (0.10 × importance)
             + (0.05 × recency_decay)
```

`recency_decay = exp(-0.010 × days_since_created)`. Episodic and working memories decay faster (λ = 0.015); semantic and procedural decay slower (λ = 0.005).

**Knowledge score:**
```
knowledge_score = (0.65 × cosine_similarity)
                + (0.15 × bm25_score)
                + (0.10 × authority_boost)
                + (0.10 × endorsement_factor)
```

Knowledge does not decay. `authority_boost` is based on visibility scope:

| Visibility | authority_boost |
|------------|----------------|
| `private` | 0.05 |
| `project` | 0.10 |
| `team` | 0.20 |
| `department` | 0.30 |
| `company` | 0.40 |

`endorsement_factor = min(endorsement_count / 10, 1.0)` — capped at 1.0 at 10 endorsements.

**Skill score:**
```
skill_score = (0.65 × cosine_similarity)
            + (0.15 × bm25_score)
            + (0.10 × authority_boost)      -- same visibility-based table as knowledge
            + (0.10 × adoption_factor)
```

`adoption_factor = min(invocation_count / 50, 1.0)` — a skill used 50+ times scores at maximum adoption. This surfaces battle-tested skills over freshly published ones.

### Step 3 — Cross-layer merge and re-rank

Memory, knowledge, and skill candidates are merged into a single list and sorted by their respective scores. Each result carries a `layer` field. Scores are normalised to `[0, 1]` and directly comparable across layers.

### Step 4 — Deduplication

If a memory was promoted to a knowledge artifact (tracked via `memories.promoted_to`), only the knowledge version is returned — the source memory is suppressed to avoid duplicate context.

---

## Knowledge Visibility and Sharing

### Visibility levels

Knowledge visibility is **explicit and intentional**. When an author publishes an artifact, they choose exactly who can see it. There is no automatic fan-out. This is the critical difference from memory scoping.

| Level | Who can read |
|-------|-------------|
| `private` | Only the owning scope (e.g., the project team) |
| `project` | All members of the project scope |
| `team` | All members of the owning team and its projects |
| `department` | All principals in the department (all teams under it) |
| `company` | Every principal in the company |

An agent querying for context at `project:acme/api` will receive:
- All `published` knowledge with `visibility=project` owned by that project
- All `published` knowledge with `visibility=team` owned by the project's parent team
- All `published` knowledge with `visibility=department` owned by the parent department
- All `published` knowledge with `visibility=company` owned by the company

This resolution is performed via the scope hierarchy using a single recursive CTE query.

### Sharing grants

For cases where visibility levels are too coarse — e.g., a developer wants to share one specific memory with their team, or a team wants to share a draft artifact with a sister team for review — fine-grained `sharing_grants` allow targeted sharing without changing the artifact's base visibility or moving it between scopes.

Grants are:
- **Directional** — from a specific item to a specific grantee scope.
- **Optionally expiring** — a grant can have an `expires_at` timestamp.
- **Optionally reshareable** — the grantee can be allowed to grant access to further scopes.
- **Audited** — the granting principal is recorded.

### Collections as organization units

Collections are particularly useful at the team and department level. Examples:

| Collection | Visibility | Owner scope |
|-----------|-----------|------------|
| `company-values` | `company` | `company:acme` |
| `security-policies` | `company` | `department:security` |
| `engineering-standards` | `department` | `department:engineering` |
| `platform-runbooks` | `team` | `team:platform` |
| `payments-architecture` | `project` | `project:acme/api` |

An agent loading context for `project:acme/api` would automatically surface artifacts from all of the above collections through the visibility resolution chain.

---

## Promotion Workflow

The promotion pathway is the bridge between the two layers. It allows agent-observed memories — potentially valuable but unreviewed — to be elevated into authoritative knowledge.

```
                   agent writes
                       │
                  [memory: draft]
                       │
         developer nominates via promote()
                       │
             [promotion_request: pending]
                       │
          ┌────────────┴───────────────┐
          │                             │
    approved by                    rejected
    required reviewers              (memory stays)
          │
  [knowledge_artifact: in_review]
          │
   endorsements accumulate
   (threshold met OR manual approve)
          │
  [knowledge_artifact: published]
          │
   visible to chosen audience,
   appears in recall + context
```

### Promotion request lifecycle

1. **Nomination** — any principal with read access to a memory can nominate it. They propose a title, target visibility level, optional collection, and a note explaining why it should be knowledge.
2. **Review assignment** — the server determines required reviewers based on the target visibility: team leads for `team`, department admins for `department`, company admins for `company`.
3. **Review** — a reviewer approves (creating the artifact as `in_review`) or rejects (with an explanatory note, memory is unchanged).
4. **Endorsement** — once in review, the artifact collects endorsements from any principal who can read it. When `endorsement_count >= review_required`, it auto-publishes. Alternatively a reviewer can manually publish.
5. **Link-back** — `memories.promoted_to` is set to the artifact ID, and `memories.promotion_status` becomes `promoted`. The memory is now suppressed in retrieval in favour of the artifact.

### Who can review

| Target visibility | Required reviewer kind |
|-------------------|----------------------|
| `project` | project owner or team admin |
| `team` | team owner or admin |
| `department` | department admin |
| `company` | company admin |

Knowledge can also be authored directly by a human developer (bypassing the promotion path) via `publish()`. Direct publishing still requires the artifact to go through the `draft → in_review → published` lifecycle unless the author has `admin` role in the target scope.

---

## Agent Integration Guide

### Claude Code via MCP

Add to `~/.claude/settings.json` (or a project-level `.claude/settings.json`):

```jsonc
{
  "mcpServers": {
    "postbrain": {
      "type": "sse",
      "url": "http://localhost:7433/mcp",
      "headers": {
        "Authorization": "Bearer <token>"
      }
    }
  }
}
```

Once connected, Claude Code automatically has access to `remember`, `recall`, `forget`, `context`, and `summarize` as native tools. The agent can call them transparently in its reasoning loop.

**Recommended hooks** (in `settings.json`):

```jsonc
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write|Bash",
        "command": "postbrain-hook snapshot --scope project:$POSTBRAIN_SCOPE"
      }
    ],
    "Stop": [
      {
        "command": "postbrain-hook summarize-session --scope project:$POSTBRAIN_SCOPE"
      }
    ]
  }
}
```

This ensures that every file change and session end triggers automatic memory updates without the agent having to remember to call `remember` explicitly.

**Skill auto-install on session start:**

Add a `PreToolUse` hook that fires once at the beginning of each session to sync skills for the current scope:

```jsonc
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write|Bash",
        "command": "postbrain-hook snapshot --scope project:$POSTBRAIN_SCOPE"
      }
    ],
    "Stop": [
      {
        "command": "postbrain-hook summarize-session --scope project:$POSTBRAIN_SCOPE"
      }
    ]
  }
}
```

And run skill sync separately as a one-time setup step or via a dedicated startup hook:

```bash
# Install all published skills visible to your scope into .claude/commands/
postbrain-hook skill sync --scope project:$POSTBRAIN_SCOPE --agent claude-code

# Or install a specific skill by slug
postbrain-hook skill install review-pr --scope team:platform
```

The `sync` command compares the installed `.md` files against the registry and installs new skills, updates changed ones (version bump), and reports deprecated ones. It is idempotent — safe to run on every session start.

### OpenAI Codex / Custom Agents via REST

```python
import httpx

POSTBRAIN = "http://localhost:7433"
HEADERS   = {"Authorization": "Bearer <token>", "Content-Type": "application/json"}

def recall(query: str, scope: str, limit: int = 8) -> list[dict]:
    r = httpx.get(f"{POSTBRAIN}/v1/memories/recall",
                  params={"q": query, "scope": scope, "limit": limit},
                  headers=HEADERS)
    r.raise_for_status()
    return r.json()["memories"]

def remember(content: str, scope: str, memory_type: str = "semantic", **kwargs):
    r = httpx.post(f"{POSTBRAIN}/v1/memories",
                   json={"content": content, "scope": scope,
                         "memory_type": memory_type, **kwargs},
                   headers=HEADERS)
    r.raise_for_status()
    return r.json()
```

---

## Implementation Plan

### Stack

| Layer | Technology | Rationale |
|-------|-----------|-----------|
| Language | Go | Low-latency server, easy static binary deployment, strong stdlib |
| HTTP framework | `net/http` + `chi` router | Lightweight, no magic |
| MCP server | `github.com/mark3labs/mcp-go` | MCP 2024-11 compliant |
| Database driver | `pgx/v5` | Native PostgreSQL protocol, prepared statements, pgvector support |
| Migrations | `golang-migrate` | Version-controlled schema evolution |
| Embedding (local) | Ollama HTTP API (`nomic-embed-text`) | No external dependency, 768-dim |
| Embedding (remote) | OpenAI `text-embedding-3-small` | 1536-dim, higher quality |
| Job scheduler | `robfig/cron` | In-process cron for housekeeping jobs |
| Config | `viper` | YAML + env var overlay |
| Observability | `log/slog` + Prometheus metrics | Structured logs, metrics endpoint |

### Directory Structure

```
postbrain/
├── cmd/
│   ├── postbrain/              # main server binary
│   │   └── main.go
│   └── postbrain-hook/         # CLI helper for agent hooks (thin REST client)
│       └── main.go
├── internal/
│   ├── api/
│   │   ├── mcp/                # MCP server + tool handlers
│   │   │   ├── server.go
│   │   │   ├── remember.go
│   │   │   ├── recall.go       # unified recall (memory + knowledge + skills)
│   │   │   ├── forget.go
│   │   │   ├── context.go
│   │   │   ├── summarize.go
│   │   │   ├── publish.go      # knowledge: publish
│   │   │   ├── endorse.go      # knowledge: endorse
│   │   │   ├── promote.go      # knowledge: promote memory→knowledge
│   │   │   ├── collect.go      # knowledge: collection management
│   │   │   ├── skill_search.go # skill: search
│   │   │   ├── skill_install.go# skill: install to agent command dir
│   │   │   └── skill_invoke.go # skill: record invocation telemetry
│   │   └── rest/               # REST API handlers
│   │       ├── router.go
│   │       ├── memories.go
│   │       ├── knowledge.go
│   │       ├── collections.go
│   │       ├── skills.go
│   │       ├── sharing.go
│   │       ├── promotions.go
│   │       ├── orgs.go         # principal + scope management
│   │       ├── sessions.go
│   │       └── graph.go
│   ├── db/
│   │   ├── conn.go             # pgx pool setup
│   │   ├── queries/            # sqlc-generated query code
│   │   │   ├── memories.sql
│   │   │   ├── knowledge.sql
│   │   │   ├── collections.sql
│   │   │   ├── principals.sql
│   │   │   ├── scopes.sql
│   │   │   └── sharing.sql
│   │   └── migrations/
│   ├── embedding/
│   │   ├── interface.go        # Embedder interface
│   │   ├── classifier.go       # content_kind heuristic (text vs code)
│   │   ├── ollama.go
│   │   ├── openai.go
│   │   └── voyage.go           # Voyage AI (voyage-code-3 for code embeddings)
│   ├── memory/
│   │   ├── store.go            # memory CRUD
│   │   ├── recall.go           # hybrid retrieval (memory side)
│   │   ├── scope.go            # scope fan-out resolution
│   │   └── consolidate.go      # dedup / merge logic
│   ├── knowledge/
│   │   ├── store.go            # knowledge artifact CRUD
│   │   ├── recall.go           # hybrid retrieval (knowledge side)
│   │   ├── visibility.go       # visibility resolution via principal chain
│   │   ├── lifecycle.go        # draft→review→published→deprecated transitions
│   │   ├── collections.go      # collection management
│   │   └── promote.go          # promotion request handling
│   ├── skills/
│   │   ├── store.go            # skill CRUD
│   │   ├── recall.go           # hybrid retrieval (skill side)
│   │   ├── install.go          # materialise skill body to agent command dir
│   │   ├── lifecycle.go        # draft→review→published→deprecated transitions
│   │   └── sync.go             # bulk sync for postbrain-hook skill sync
│   ├── retrieval/
│   │   └── merge.go            # cross-layer merge and re-rank (memory+knowledge+skills)
│   ├── principals/
│   │   ├── store.go            # principal CRUD
│   │   └── membership.go       # membership resolution
│   ├── sharing/
│   │   └── grants.go           # sharing grant CRUD + access check
│   ├── graph/
│   │   └── relations.go        # entity + relation management
│   ├── jobs/
│   │   ├── scheduler.go
│   │   ├── expire.go           # TTL cleanup for working memory
│   │   ├── consolidate.go      # periodic near-duplicate merging
│   │   ├── reembed.go          # re-embed on model change (text + code, independent)
│   │   ├── staleness.go        # signal 2: contradiction detection (weekly LLM job)
│   │   └── promotion_notify.go # notify reviewers of pending promotions
│   └── config/
│       └── config.go
├── migrations/
│   ├── 001_initial_schema.up.sql
│   ├── 001_initial_schema.down.sql
│   ├── 002_knowledge_layer.up.sql
│   ├── 002_knowledge_layer.down.sql
│   ├── 003_age_graph.up.sql
│   ├── 003_age_graph.down.sql
│   ├── 004_multi_model_embeddings.up.sql
│   ├── 004_multi_model_embeddings.down.sql
│   ├── 005_skills.up.sql
│   └── 005_skills.down.sql
├── config.example.yaml
├── docker-compose.yml
├── Makefile
├── go.mod
└── DESIGN.md
```

### Key Implementation Notes

#### Embedding dimensions are configurable

The `embedding` column uses `vector(1536)` in the schema above to match OpenAI's `text-embedding-3-small`. When using Ollama's `nomic-embed-text` (768-dim), the server is configured with `embedding.dimensions: 768` and migrations create the column accordingly. Switching embedding models requires a `reembed` migration job that re-embeds all existing memories.

#### sqlc for type-safe queries

All database access goes through `sqlc`-generated code. Raw SQL lives in `db/queries/*.sql`; the Go structs and functions are generated at build time. This eliminates `interface{}` scanning and makes schema changes visible as compile errors.

#### Consolidation algorithm

The consolidation job runs every 6 hours from the Go server (requires LLM calls, so it cannot live in pg_cron):

1. For each scope, find embedding clusters: pairs of memories whose cosine distance is ≤ 0.05 (pure SQL against the HNSW index).
2. Within each cluster, if all members have `importance < 0.7` and `access_count < 3`, submit content to the LLM summarizer, write the synthesized memory, soft-delete the originals, and record the merge in `consolidations`.

**Entity deduplication** runs as part of the same job using `fuzzystrmatch`. After memory consolidation, the job scans the entity registry for near-duplicate names within the same scope:

```sql
-- Find candidate duplicate entities using Levenshtein + Metaphone + trigram
SELECT a.id, b.id, a.name, b.name,
       levenshtein(lower(a.name), lower(b.name))     AS edit_dist,
       metaphone(a.name, 10) = metaphone(b.name, 10) AS sounds_alike,
       similarity(a.name, b.name)                     AS trgm_sim
FROM   entities a
JOIN   entities b
       ON  a.scope_id    = b.scope_id
       AND a.entity_type = b.entity_type
       AND a.id          < b.id   -- avoid symmetric pairs
WHERE  levenshtein(lower(a.name), lower(b.name)) <= 2
    OR (    metaphone(a.name, 10) = metaphone(b.name, 10)
        AND similarity(a.name, b.name) > 0.6);
```

Each function catches a different class of duplicate: `pg_trgm`'s `similarity()` for partial/substring matches ("PaymentSvc" vs "payment-service"), `levenshtein` for typos and abbreviations, `metaphone` for phonetic variants across languages. Candidates above threshold are merged Go-side: relations and memory links are re-pointed to the canonical entity, the duplicate is deleted.

#### Decay scoring

Decay is handled entirely by the `decay-memory-importance` pg_cron job defined in the schema — no application server uptime required. The formula:

```
new_importance = importance × exp(−λ × days_since_last_access)
```

λ per type: `0.015` working, `0.010` episodic, `0.005` semantic/procedural. The weekly `prune-low-value-memories` pg_cron job handles soft-deletion of memories where `importance < 0.05` and `access_count < 2`.

#### Multi-model embedding

The embedding service inspects each write to determine `content_kind`:

1. If `source_ref` starts with `file:` or `git:`, it's `code`.
2. Otherwise, a lightweight heuristic (ratio of code-like tokens — brackets, operators, keywords) classifies the content. Threshold is configurable; defaults to treating content as `text` when ambiguous.

On a `code` write, the service calls both the active text model (for cross-content recall) and the active code model (for code-specific recall), populating `embedding` and `embedding_code` respectively.

**Retrieval model selection** is driven by `search_mode` in the query:

| `search_mode` | Index used | Typical caller |
|---------------|-----------|----------------|
| `auto` (default) | text HNSW; also code HNSW if query looks like code | agents |
| `text` | text HNSW only | marketing / content agents |
| `code` | code HNSW only | IDE integrations, code review agents |
| `both` | both HNSW, results merged by score | broad context gathering |

In `auto` mode, the same heuristic used at write time is applied to the query string. If the query is classified as code, results from both indexes are fetched and merged; the code-HNSW results get a +0.05 score boost since the caller is likely in a coding context.

**Model transition (reembed job):** when the active model for a `content_type` changes, the Go `reembed` job:
1. Queries all rows where `embedding_model_id != new_active_model_id` (for text) or `embedding_code_model_id != new_active_code_model_id` (for code).
2. Re-embeds in batches of 100, back-filling the column.
3. Writes are dual — the old embedding stays in place until the batch for that row completes, so queries during migration still return results (with slightly degraded cross-batch similarity, acceptable for ANN).
4. Text and code reembed jobs run independently; upgrading the code model doesn't touch text embeddings.

#### Staleness detection

Three signals, each with distinct triggering, confidence, and cost profile.

**Signal 1 — `source_modified` (confidence: 0.9, cost: zero)**

Triggered by the `PostToolUse` hook in real time. When the Go server receives a hook event after an agent Edit/Write/Bash:

```go
// Extract modified file paths from the event payload, then:
SELECT id FROM knowledge_artifacts
WHERE  status     = 'published'
AND    source_ref = ANY($modified_file_paths::text[]);
// → create staleness_flag for each hit
```

No embeddings, no LLM. Direct causal evidence: the exact file a knowledge artifact describes just changed.

**Signal 2 — `contradiction_detected` (confidence: 0.8, cost: LLM calls)**

Weekly Go job. Two-phase to minimise LLM spend:

*Phase 1 — cheap pre-filter using negation embeddings:*
```sql
-- For each published artifact, embed its negation (once, cached in artifact.meta)
-- then find recent memories that are suspiciously close to that negation.
SELECT ka.id AS artifact_id, m.id AS memory_id,
       1 - (ka.embedding <=> m.embedding) AS positive_sim,
       1 - ($negation_embedding <=> m.embedding) AS negation_sim
FROM   knowledge_artifacts ka
JOIN   memories m ON m.scope_id = ka.owner_scope_id
WHERE  ka.status    = 'published'
AND    m.created_at > now() - INTERVAL '30 days'
AND    m.is_active  = true
-- Semantically close to the artifact (same topic)...
AND    (1 - (ka.embedding <=> m.embedding)) > 0.75
-- ...but also suspiciously close to its negation (possible contradiction)
AND    (1 - ($negation_embedding <=> m.embedding)) > 0.60;
```

The negation embedding is computed once as `embed("It is NOT the case that: " + artifact.summary)` and stored in `knowledge_artifacts.meta->>'negation_embedding_cache'`. It is invalidated when the artifact content changes.

*Phase 2 — LLM classifier on surviving pairs only:*
```
Prompt: "Does the following MEMORY contradict the KNOWLEDGE ARTIFACT?
Answer exactly one of: CONSISTENT / EXTENDS / CONTRADICTS
KNOWLEDGE ARTIFACT: {artifact.content}
MEMORY: {memory.content}"
```

Only pairs that pass the negation pre-filter reach the LLM, reducing API calls by ~80–90% in practice. `CONTRADICTS` verdicts create a staleness flag with the memory IDs and classifier reasoning in `evidence`.

**Signal 3 — `low_access_age` (confidence: 0.3, cost: zero)**

Monthly pg_cron job (SQL above in schema). The lowest-confidence signal — an unread artifact isn't necessarily wrong, just potentially unknown or orphaned. Included as a catch-all for documentation that has simply been forgotten.

**How staleness surfaces in API responses:**

```jsonc
// recall result with active staleness flag
{
  "layer":    "knowledge",
  "id":       "0194ab3c-...",
  "title":    "Payment Service Architecture",
  "score":    0.94,
  "staleness": {
    "flagged":           true,
    "signals":           ["source_modified", "contradiction_detected"],
    "highest_confidence": 0.9,
    "flagged_at":        "2026-03-21T09:14:00Z"
  }
}
```

Agents are expected to surface the warning in their reasoning ("⚠ this artifact may be outdated — verify before relying on it"). The artifact is **not** score-penalised automatically; that decision is left to the reviewing human. Dismissing a flag (`POST /v1/knowledge/stale/:id/dismiss`) records the reviewer's confirmation that the artifact is still valid.

New REST endpoints:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/knowledge/stale` | Open flags, sorted by confidence desc |
| `POST` | `/v1/knowledge/stale/:id/dismiss` | Mark as still valid (with note) |
| `POST` | `/v1/knowledge/stale/:id/resolve` | Mark as fixed (artifact deprecated or updated) |

#### Authentication & multi-tenancy

Each token is scoped to a `(principal_id, allowed_scope_ids[])` pair stored in a `tokens` table. Middleware validates the token and injects the resolved scope IDs into the request context. Cross-scope reads are only allowed within the scope hierarchy owned by the token.

---

## Deployment

### Docker Compose (local / self-hosted)

```yaml
# docker-compose.yml
services:
  postgres:
    # pg_vector publishes images for each PG major; use 18 once available.
    # pg_cron, pg_partman, ltree, citext, unaccent, fuzzystrmatch, pg_trgm,
    # btree_gin, and pg_prewarm ship with standard PostgreSQL.
    # pg_partman must be installed separately (see Dockerfile.postgres).
    image: pgvector/pgvector:pg18
    environment:
      POSTGRES_DB:       postbrain
      POSTGRES_USER:     postbrain
      POSTGRES_PASSWORD: postbrain
    ports: ["5432:5432"]
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./postgres/postgresql.conf:/etc/postgresql/postgresql.conf:ro
    command: postgres -c config_file=/etc/postgresql/postgresql.conf

  postbrain:
    build: .
    depends_on: [postgres]
    ports:
      - "7433:7433"
    environment:
      POSTBRAIN_DATABASE_URL: postgres://postbrain:postbrain@postgres:5432/postbrain
      POSTBRAIN_EMBEDDING_BACKEND: ollama
      POSTBRAIN_EMBEDDING_OLLAMA_URL: http://host.docker.internal:11434
    volumes:
      - ./config.yaml:/etc/postbrain/config.yaml:ro

volumes:
  pgdata:
```

**`postgres/postgresql.conf` (minimum required settings):**

```ini
# Extensions that must be loaded at server start
shared_preload_libraries = 'pg_cron,pg_partman_bgw,pg_prewarm'

# pg_cron: run jobs as the postbrain superuser
cron.database_name = 'postbrain'

# HNSW index builds need more working memory
maintenance_work_mem = '2GB'

# pg_prewarm: restore buffer cache state across restarts (optional but useful)
pg_prewarm.autoprewarm = on
```

**`Dockerfile.postgres`** (if building a custom image with pg_partman):

```dockerfile
FROM pgvector/pgvector:pg18
RUN apt-get update \
 && apt-get install -y postgresql-18-partman \
 && rm -rf /var/lib/apt/lists/*
```

### Production considerations

- Run PostgreSQL on a dedicated instance. HNSW index builds require `maintenance_work_mem` ≥ 2 GB; keep `shared_buffers` large enough to hold the three HNSW indexes hot (~RAM/4 as a starting point).
- Use connection pooling via PgBouncer in **transaction mode**. `pg_cron` requires a persistent background worker connection; keep one dedicated non-pooled connection for it.
- The Postbrain server is stateless; horizontal scaling works out of the box — all state is in PostgreSQL.
- pg_prewarm with `autoprewarm = on` saves the buffer map to disk on shutdown and restores it on the next startup, avoiding the cold-start HNSW latency spike after a planned restart.
- Back up with `pg_dump --format=custom`; the embedding vector columns are large. For large deployments, use logical replication to a read replica for point-in-time recovery without backup windows.
- The `events` table is partitioned monthly. Old partitions can be detached (`ALTER TABLE events DETACH PARTITION events_2026_01`) and archived to cold storage without any downtime or locking.

---

## Key Implementation Notes

### Visibility resolution query

The most frequently executed non-trivial query is resolving which knowledge artifacts a principal can read. With `ltree`, all ancestor scope lookups become index-range scans on the GiST index — no recursive CTE required.

The `@>` operator means *"is ancestor of or equal to"*: `'acme.engineering'::ltree @> 'acme.engineering.platform.acme_api'::ltree` is `true`.

```sql
-- Resolve visible knowledge artifacts for a principal querying from $project_path.
-- $project_path is the ltree path of the project scope, e.g.:
--   'acme.engineering.platform.acme_api'
--
-- The GiST index on scopes.path makes the @> operator a fast index scan.

SELECT ka.*
FROM   knowledge_artifacts ka
JOIN   scopes s ON ka.owner_scope_id = s.id
WHERE  ka.status = 'published'
AND (
    -- project visibility: artifact's owner scope IS the queried project
    (ka.visibility = 'project'
        AND s.path = $project_path)

    -- team visibility: artifact's owner scope is a team ancestor of the project
    OR (ka.visibility = 'team'
        AND s.path @> $project_path
        AND s.kind = 'team')

    -- department visibility: artifact's owner scope is a department ancestor
    OR (ka.visibility = 'department'
        AND s.path @> $project_path
        AND s.kind = 'department')

    -- company visibility: artifact's owner scope is the company root
    OR (ka.visibility = 'company'
        AND s.kind = 'company')

    -- explicit sharing grant to any ancestor scope (including the project itself)
    OR ka.id IN (
        SELECT sg.artifact_id
        FROM   sharing_grants sg
        JOIN   scopes gs ON sg.grantee_scope_id = gs.id
        WHERE  (gs.path @> $project_path OR gs.path = $project_path)
        AND    (sg.expires_at IS NULL OR sg.expires_at > now())
    )
);
```

This query is executed once per `recall` call. Results are then filtered through the HNSW ANN search via a CTE materialization or a temporary table, avoiding repeated visibility checks during vector scoring.

### Preventing scope escalation on writes

The auth middleware enforces that the `scope` field in a `remember` or `publish` call must resolve to a scope the caller's token covers. Tokens are scoped to a `(principal_id, allowed_scope_ids[])` pair. Attempting to write to a scope not in that list returns 403 before any DB access.

### Knowledge artifact versioning

When a published artifact is edited, the current version is snapshotted into `knowledge_history` before the update is applied. The `version` counter increments. The HNSW index is updated with the new embedding asynchronously (via an in-process job queue) to avoid write latency on the critical path.

---

## PostgreSQL Extensions Reference

All extensions are standard PostgreSQL contrib modules or well-established third-party packages. None require external services.

| Extension | Source | Role in Postbrain |
|-----------|--------|-------------------|
| **`vector`** (pg_vector) | Third-party | HNSW indexes and cosine/L2 distance operators on `embedding vector(N)` columns. The core of semantic search. |
| **`ltree`** | contrib | Hierarchical label-tree paths on `scopes.path`. Replaces recursive CTEs for ancestor/descendant queries with a single GiST index scan using the `@>` / `<@` operators. |
| **`pg_trgm`** | contrib | Trigram similarity index (`gin_trgm_ops`) for fuzzy keyword search and the `similarity()` scoring function used in entity deduplication. Paired with the HNSW pass for hybrid recall. |
| **`btree_gin`** | contrib | Allows GIN multi-column indexes to include btree-indexable types (UUIDs, booleans). Used on composite GIN indexes that cover both text and non-text predicates. |
| **`unaccent`** | contrib | Strips diacritical marks before stemming. Registered in the `postbrain_fts` text search configuration so "résumé", "resume", and "Resume" all produce the same FTS tokens — important for international teams. |
| **`citext`** | contrib | Case-insensitive text type. Applied to `principals.slug`, `scopes.external_id`, and `knowledge_collections.slug` so that lookups for `"Alice@Acme.com"` and `"alice@acme.com"` resolve to the same row without `LOWER()` wrappers or functional indexes. |
| **`fuzzystrmatch`** | contrib | `levenshtein()` for edit-distance (typos, abbreviations), `metaphone()` for phonetic matching (cross-language name variants). Used in the Go-side entity deduplication job alongside `pg_trgm`'s `similarity()`. |
| **`pg_prewarm`** | contrib | Loads index pages into `shared_buffers` on demand or automatically on startup (`autoprewarm = on`). Prevents the cold-start HNSW latency spike after a planned restart — without it, the first few hundred queries after restart pay full disk I/O cost. |
| **`pg_cron`** | Third-party | In-database cron scheduler. Runs TTL expiry (every 5 min), importance decay (nightly), low-value memory pruning (weekly), and pg_partman maintenance (hourly) as background SQL jobs. Decouples housekeeping from application server uptime. |
| **`pg_partman`** | Third-party | Automated time-range partition management for the `events` table (monthly partitions). Pre-creates upcoming partitions, optionally detaches old ones for archival. Keeps query plans and vacuum efficient as the audit log grows. |

> **Note on Apache AGE:** listed in the Knowledge Graph section rather than here; it is an optional overlay, not a baseline dependency.

### Not included and why

| Extension | Reason excluded |
|-----------|----------------|
| **`pgcrypto`** | UUID generation is native in PG 18 (`uuidv7()`). Token hashing belongs in the application layer (`crypto/sha256` in Go) — plaintext tokens should never reach the database. |
| **`pg_stat_statements`** | Valuable for production query profiling but operational, not schema-level. Enable in `postgresql.conf` (`shared_preload_libraries`) independently of the application schema. |
| **`pgaudit`** | Compliance-level audit logging. Recommended for regulated environments but adds significant write amplification; enable per-deployment need rather than by default. |

---

## Knowledge Graph

### What is already a knowledge graph

The `entities` + `relations` + `memory_entities` tables form a **labeled property graph stored in relational tables**. Nodes (`entities`) carry typed attributes, embeddings, and a canonical identifier. Directed edges (`relations`) carry a typed predicate, a confidence score, and a provenance link back to the source memory. This *is* a knowledge graph by definition; it just doesn't have a dedicated query language or traversal engine.

For the access patterns that cover the majority of agent queries — 1–2 hop lookups such as "what does the auth service own?", "who authored this file?", "what does this module depend on?" — the current relational model with simple joins is sufficient and fast.

### Where it falls short

| Capability | Current design | Gap |
|------------|----------------|-----|
| Multi-hop traversal | Recursive CTE with depth limit | Degrades past 3–4 hops; no shortest-path or subgraph queries |
| Impact analysis | Manual recursive CTE | Cannot efficiently answer "what transitively depends on X?" across large graphs |
| Transitive inference | Not supported | Cannot derive `A→C` from `A→B` + `B→C` without materialising it |
| Graph-weighted scoring | Flat entity importance | No PageRank or centrality signal to boost architecturally critical nodes in retrieval |
| Domain subgraphs | Not supported | Cannot ask "give me all entities in the payments domain and their relationships" as a graph query |
| Cypher / GQL queries | Not supported | No expressive path-pattern language |

### Extension: Apache AGE as an overlay

[Apache AGE](https://age.apache.org/) is a PostgreSQL extension that adds a full ISO GQL / openCypher property graph model — stored **inside** PostgreSQL tables. No external graph database is required; everything stays in the same instance.

The design decision is: **AGE is an overlay, not a replacement**. The `entities` and `relations` tables remain the source of truth and the write path. A background sync job mirrors them into an AGE graph for traversal queries. This keeps write latency on the main path unchanged and makes AGE optional — deployments that don't need multi-hop traversal simply don't enable it.

#### Schema additions

```sql
-- Load AGE (must be in shared_preload_libraries)
CREATE EXTENSION IF NOT EXISTS age;
LOAD 'age';
SET search_path = ag_catalog, "$user", public;

-- Create the AGE graph (one per Postbrain instance; scoping is handled by
-- entity/relation metadata, not by separate graphs)
SELECT create_graph('postbrain');
```

AGE stores graph data in its own internal tables under the graph name. The sync job manages population.

#### Sync job

A Go background worker (runs every 15 minutes, or triggered after consolidation) mirrors the relational graph into AGE:

```sql
-- Upsert entity nodes into the AGE graph
-- AGE vertices carry the same id as the relational entities row.
SELECT * FROM cypher('postbrain', $$
    MERGE (e:Entity {id: $id})
    SET e.entity_type = $entity_type,
        e.name        = $name,
        e.canonical   = $canonical,
        e.scope_id    = $scope_id
$$, $params) AS (result agtype);

-- Upsert relation edges
SELECT * FROM cypher('postbrain', $$
    MATCH (a:Entity {id: $subject_id}), (b:Entity {id: $object_id})
    MERGE (a)-[r:RELATION {predicate: $predicate}]->(b)
    SET r.confidence = $confidence,
        r.scope_id   = $scope_id
$$, $params) AS (result agtype);
```

Deletions are propagated by the same job by comparing entity/relation `updated_at` timestamps against the last sync checkpoint stored in a `graph_sync_state` table.

#### New API surface

A `graph_query` MCP tool and REST endpoint expose Cypher traversal to agents:

```jsonc
// MCP: graph_query
// Input
{
  "cypher": "MATCH (e:Entity {canonical: 'src/payments/stripe.go'})-[:DEPENDS_ON*1..3]->(dep) RETURN dep",
  "scope":  "project:acme/api",
  "limit":  20
}

// Output
{
  "nodes": [
    { "id": "...", "entity_type": "file",    "name": "src/payments/client.go" },
    { "id": "...", "entity_type": "service", "name": "stripe-api"             }
  ],
  "edges": [
    { "from": "...", "to": "...", "predicate": "depends_on", "confidence": 0.95 }
  ]
}
```

Scope enforcement is applied before query execution: the Cypher query is wrapped in a scope filter that restricts traversal to entities whose `scope_id` is reachable by the caller.

REST: `POST /v1/graph/query` with the same payload.

#### PageRank for entity importance

Once AGE is in place, a weekly pg_cron job computes PageRank over the graph and writes scores back to `entities.importance`:

```sql
-- pg_cron weekly job: compute PageRank via AGE and write back to relational table
SELECT cron.schedule('pagerank-entities', '0 5 * * 1', $$
    WITH ranked AS (
        SELECT id, rank
        FROM age_pagerank('postbrain', 'Entity', 'RELATION', 0.85, 20)
    )
    UPDATE entities e
    SET    meta = jsonb_set(meta, '{pagerank}', to_jsonb(r.rank))
    FROM   ranked r
    WHERE  e.id = r.id::uuid
$$);
```

The `pagerank` value in `entities.meta` is then incorporated into the entity-boosted component of the retrieval scoring formula, so memories and knowledge artifacts linked to high-centrality entities (e.g. a shared authentication library that 30 services depend on) score higher when queried from any of those services' scopes.

#### Impact analysis tool

The primary agent-facing use case for multi-hop traversal is **change impact analysis**: before editing a file or service, the agent asks what else is affected.

```jsonc
// MCP: graph_impact  (new tool)
// "What would be affected if I change src/auth/jwt.go?"
{
  "entity":    "file:src/auth/jwt.go",
  "scope":     "project:acme/api",
  "direction": "inbound",   // who depends on this entity
  "max_depth": 4,
  "predicates": ["depends_on", "imports", "extends"]
}

// Output: impact rings, closest first
{
  "rings": [
    { "depth": 1, "entities": ["src/middleware/auth.go", "src/handlers/user.go"] },
    { "depth": 2, "entities": ["src/api/router.go", "src/handlers/payment.go"] },
    { "depth": 3, "entities": ["cmd/server/main.go"] }
  ],
  "total_affected": 5
}
```

This is the kind of query that takes one Cypher `MATCH` clause but would require a complex, depth-limited recursive CTE without AGE.

#### When to enable AGE

AGE adds operational complexity (an additional shared library, a sync job, a larger `shared_preload_libraries`). The recommended rollout:

| Phase | Capability | AGE needed? |
|-------|-----------|-------------|
| Initial | Memory recall, knowledge publishing, 1-hop entity lookups | No |
| Growth | Impact analysis, domain subgraph queries | Yes |
| Scale | PageRank-weighted retrieval, Cypher ad-hoc queries from developers | Yes |

For phase 1 deployments, the `relations` table and depth-limited recursive CTEs are sufficient. The schema is forward-compatible: nothing needs to change when AGE is added, because it reads from `entities`/`relations` via the sync job rather than replacing them.

#### Directory additions

```
internal/
  graph/
    relations.go        # existing: relational CRUD
    age_sync.go         # new: sync entities+relations → AGE graph
    age_query.go        # new: execute scoped Cypher queries via AGE
    pagerank.go         # new: weekly PageRank computation + writeback
  api/
    mcp/
      graph_query.go    # new: graph_query MCP tool
      graph_impact.go   # new: graph_impact MCP tool
    rest/
      graph.go          # extended: add /v1/graph/query endpoint
migrations/
  003_age_graph.up.sql
  003_age_graph.down.sql
```

---

## Open Questions / Future Work

| Topic | Notes |
|-------|-------|
| **Federated instances** | Allow multiple Postbrain deployments to sync company- or department-level knowledge (e.g., between subsidiaries or open-source project contributor communities). |
| **Agent-specific embedding models** | ✅ Resolved — `embedding_models` registry with separate text/code active models; `embedding_code` column on memories; independent `reembed` jobs per content type. |
| **Streaming recall** | Server-sent events for streaming large recall result sets into context windows incrementally. |
| **Review notifications** | Email / Slack / webhook notifications when a promotion request is pending review. Currently just polled via REST. |
| **CLI / TUI** | `postbrain ls`, `postbrain search`, `postbrain knowledge browse` for human inspection without the web UI. |
| **Web UI** | Visual knowledge browser: collection view, entity graph, promotion review queue, per-team knowledge health metrics (stale artifacts, low-endorsement artifacts). |
| **Knowledge staleness detection** | ✅ Resolved — three signals: source_modified (hook-triggered), contradiction_detected (weekly LLM job with negation-embedding pre-filter), low_access_age (monthly pg_cron). `staleness_flags` table; annotated in recall/context responses; review queue at `GET /v1/knowledge/stale`. |
| **Cross-company knowledge sharing** | Explicit opt-in mechanism for sharing `company`-visibility artifacts with partner companies (e.g., shared API contracts between API provider and consumer). |
| **Skill outcome tracking** | v2: a `skill_invocations` table with an outcome/rating column so teams can see which skills consistently produce good results vs which get abandoned mid-run. The `events` table covers invocation counts for v1. |
| **Skill testing** | v2: a `skill_test_cases` table with input/expected-output pairs; a CI-style job that re-runs tests when a skill is updated and blocks publishing if tests regress. |
| **Public skill marketplace** | Opt-in registry of `company`-visibility skills that organisations choose to share publicly (e.g. open-source project contributors sharing their `/triage-issue` skill). Requires a federation/trust model. |
