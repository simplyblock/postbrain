# Postbrain — Data Model Design

## Purpose

This document defines the structural model for principals, scopes, and the
three storage layers (memory, knowledge, skills), plus the main PostgreSQL
entity boundaries.

## Layer model

Postbrain separates data into three first-class layers:

- **Memory**: high-churn, session-driven observations.
- **Knowledge**: reviewed and versioned artifacts for broader reuse.
- **Skills**: executable prompt templates with parameter schemas and
  agent-compatibility metadata.

The layers are intentionally separate because they differ in trust level,
lifecycle, and publishing semantics.

## Taxonomy

### Memory types

- `semantic`
- `episodic`
- `procedural`
- `working` (TTL/expiry-capable)

### Knowledge types

- `semantic`
- `episodic`
- `procedural`
- `reference`
- `digest` (synthesized topic artifact)

### Skill attributes

- slug and body/template
- typed parameter definitions
- compatible agent types
- explicit visibility and lifecycle state

## Principal model

Principals represent all actors: users, teams, departments, companies, and
agents.

Key concepts:

- Principal hierarchy comes from membership edges.
- Access derives from principal relationships plus scope authorization logic.
- Agent principals can be attached to user/team ownership contexts.

## Scope model

Scopes are explicit write/read namespaces (for example user/team/project).

Key invariants:

- Every write targets exactly one scope.
- Read expansion can include ancestor scopes according to request mode.
- Scope lineage drives multi-level recall and policy application.

## PostgreSQL entity boundaries

Core logical groups:

- identity and access entities:
  - `principals`, `principal_memberships`, `tokens`, `scopes`
- memory and graph entities:
  - `memories`, `entities`, `relations`, `memory_entities`
- knowledge entities:
  - `knowledge_artifacts`, `knowledge_history`, `knowledge_endorsements`,
    `knowledge_collections`, `knowledge_collection_items`, `artifact_entities`
- promotion and sharing entities:
  - `promotion_requests`, `sharing_grants`
- sessions and observability entities:
  - `sessions`, `events`
- model registry entities:
  - `ai_models` (`model_type`-scoped embedding/generation registry),
    `embedding_models` (legacy compatibility), and associated embedding infrastructure

## Extension set

Primary extension family:

- vector/semantic search support (`pgvector`)
- text and fuzzy retrieval support (`pg_trgm`, `unaccent`, FTS config)
- hierarchy operators (`ltree`)
- scheduling/partition support (`pg_cron`, `pg_partman`)

## Related documents

- `designs/DESIGN_RETRIEVAL_AND_LIFECYCLE.md`
- `designs/DESIGN_API_AND_INTEGRATIONS.md`
- `designs/DESIGN_PERMISSIONS.md`
- `designs/DESIGN_CODE_GRAPH.md`
- `designs/TASKS.md`
