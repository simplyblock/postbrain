# Postbrain — Architecture Overview

This document is intentionally high-level.

Use it to understand system shape, boundaries, and core invariants.
For implementation details, read the focused design documents listed in
"Design document map" below.

## Overview

Postbrain is a persistent, queryable memory backend for coding agents and
human developers. It provides three complementary layers:

1. **Memory**: high-volume observations captured during work.
2. **Knowledge**: reviewed, versioned artifacts intended for reuse.
3. **Skills**: executable, parameterized prompt templates for agents.

All layers are queryable through a single server exposing MCP and REST,
backed by PostgreSQL.

## Architectural goals

- Durable long-term context across sessions and tools.
- Semantic retrieval with predictable low latency.
- Multi-principal, scope-aware access control.
- Explicit publishing and visibility for shared artifacts.
- Promotion path from raw memory to durable knowledge and reusable skills.
- Cross-agent interoperability (Claude Code, Codex, CI automation).

## System context

```text
Agents / Users / CI
        |
     MCP / REST
        |
  Postbrain Server
  - authn/authz
  - memory/knowledge/skills services
  - retrieval + ranking
  - background jobs
        |
    PostgreSQL
```

## Core invariants

- Writes always target an explicit scope.
- Tokens are downscoped credentials and never elevate principal access.
- Knowledge and skills are shared via explicit visibility and review state.
- Memory, knowledge, and skills remain logically separate stores.
- Retrieval can combine layers, but authorization filters always apply first.

## Design document map

Read `docs/index.md` first to choose what to read in full.

### Core design details

- `designs/DESIGN_DATA_MODEL.md`
  - Principal/scope model, layer taxonomy, PostgreSQL schema boundaries.
- `designs/DESIGN_API_AND_INTEGRATIONS.md`
  - MCP tools, REST surface, agent integration patterns.
- `designs/DESIGN_RETRIEVAL_AND_LIFECYCLE.md`
  - Retrieval/scoring, visibility, promotion workflow, synthesis/staleness.
- `designs/DESIGN_OPERATIONS.md`
  - Implementation plan, deployment and operational constraints.
- `designs/DESIGN_UX.md`
  - Web UI and TUI architecture.

### Specialized designs

- `designs/DESIGN_PERMISSIONS.md`
  - Fine-grained authorization model and inheritance rules.
- `designs/DESIGN_OAUTH.md`
  - OAuth client/server flows and schema/config impacts.
- `designs/DESIGN_CODE_GRAPH.md`
  - Code graph extraction/indexing and graph-assisted retrieval.

## Change policy

`designs/DESIGN.md` should remain concise and stable.
Detailed design changes should be made in the focused design files above.
