---
version: 1
---

# Postbrain — Codex (Hooks-Enabled)

Postbrain provides persistent memory, shared knowledge, and a skill registry via MCP.

This lightweight profile assumes repo-local Codex hooks are installed and enabled.

## Startup checklist

1. Verify Postbrain is reachable with `list_scopes`.
2. Resolve scope from local config (`.codex/postbrain-base.md`, `README.md`, `AGENTS.md`).
3. Start a session: `session_begin(scope=...)`.
4. Load task context: `context(scope=..., query=...)`.

## Scope policy

Never invent a scope. Use an explicit `kind:external_id` (for example `project:acme/api`).

If no scope is configured, ask the user and persist the answer in `.codex/postbrain-base.md`.

## Working policy

- Call `recall` before each non-trivial task.
- Because hooks are installed, snapshot/summarize memory capture is automatic:
  - `snapshot` on post-tool events
  - `summarize-session` on stop
- Still call `remember` manually for important design decisions, constraints, and rationale that should be durable.
- Use `publish`/`promote` for long-lived artifacts.

## Core tools

- `context(scope, query, max_tokens)` for bootstrap context.
- `recall(query, scope, layers, limit)` for targeted retrieval.
- `knowledge_detail(artifact_id)` when summary-only recall is insufficient.
- `remember(content, scope, memory_type, summary, entities, source_ref)` for explicit high-value memory entries.
- `publish(...)` and `promote(...)` for durable knowledge.
- `skill_search` / `skill_install` / `skill_invoke` for reusable automation.

## Session close

Hooks invoke summarize automatically, but still call `session_end(session_id)` at end of work.

## Environment

- `POSTBRAIN_URL` (default `http://localhost:7433`)
- `POSTBRAIN_TOKEN` (required)
- `POSTBRAIN_SCOPE` (recommended)
