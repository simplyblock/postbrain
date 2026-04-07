# Postbrain — API and Integrations Design

## Purpose

This document describes API surfaces and integration patterns used by agents,
CLI tooling, and automation.

## API surfaces

### MCP (primary)

MCP is the first-class interface for agent integrations.

Tool families:

- memory tools: `remember`, `recall`, `forget`, `context`, `summarize`
- knowledge tools: `publish`, `endorse`, `promote`, `collect`,
  `knowledge_detail`, `synthesize_topic`
- skill tools: `skill_search`, `skill_install`, `skill_invoke`
- discovery/utility: `list_scopes`, optional graph-query tools

### REST (fallback and platform integration)

REST mirrors core platform capabilities for:

- non-MCP automation
- CI pipelines and scripted operations
- UI backends and admin workflows

Main domains:

- memories, knowledge, skills
- scopes, principals, memberships
- promotions, sharing, collections
- graph and indexing controls

## Authentication model

- bearer-token authentication for API access
- token permissions and scope restrictions enforced by centralized authz
- tokens are downscoped and cannot elevate principal capability

For detailed permission semantics, see `designs/DESIGN_PERMISSIONS.md`.

## Integration patterns

### Claude Code

- MCP endpoint + bearer header
- optional hook automation for snapshots and end-of-session summarization
- skill install/sync flows for local command materialization

### Codex and custom agents

- REST-first integrations where MCP is unavailable
- shared scope conventions for team/project isolation
- optional skill consumption via API/CLI sync

### CI / batch jobs

- issue dedicated limited-scope tokens
- use explicit scope targeting for deterministic writes
- avoid broad visibility by default

## Pagination and response behavior

- list endpoints use cursor or offset/limit patterns consistently per surface
- API contracts should preserve stable filtering and ordering behavior

## Change policy

- Additive API changes are preferred.
- Breaking behavior changes require explicit task/design updates and migration
  notes.
- MCP and REST parity should be maintained where practical.

## Related documents

- `designs/DESIGN_RETRIEVAL_AND_LIFECYCLE.md`
- `designs/DESIGN_PERMISSIONS.md`
- `designs/DESIGN_OAUTH.md`
- `designs/TASKS.md`
