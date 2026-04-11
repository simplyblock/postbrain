---
name: postbrain
description: Postbrain Claude Code persistent memory and knowledge operating policy
version: 1
user-invocable: false
allowed-tools: Bash(postbrain-cli *)
---

# Postbrain — Persistent Memory & Knowledge for Claude Code

This skill defines the operating workflow for Postbrain in Claude Code.

## 1. Server Gate

Run `list_scopes` first.

If it fails, stop and report:

> Postbrain MCP server is not reachable. Ensure `POSTBRAIN_URL` (default `http://localhost:7433`) and `POSTBRAIN_TOKEN` are valid.

## 2. Scope Resolution

Scope format must be `kind:external_id`.

Resolve scope in this order:

1. `.claude/postbrain-base.md`
2. `.agents/postbrain-base.md`
3. `CLAUDE.md`
4. `README.md`

If unresolved, ask user and persist the chosen value to `.claude/postbrain-base.md`.
Never invent scope values.

Canonical format when persisting scope (for `.claude/postbrain-base.md` or `.agents/postbrain-base.md`):

```md
---
postbrain_enabled: true
postbrain_scope: project:acme/api
updated_at: YYYY-MM-DD
---
```

## 3. Session Bootstrap

1. `session_begin(scope=...)`
2. `context(scope=..., query=<task>, max_tokens=4000)`
3. `recall(query=<task>, scope=...)` before each non-trivial task

## 4. Hook-Aware Behavior

When Claude hooks are configured, `snapshot` and `summarize-session` run automatically.

Still call `remember` manually for:

- architecture decisions
- constraints/tradeoffs
- durable procedures

## 5. Memory & Knowledge Rules

For `remember`, always include:

- high-signal `summary`
- concrete `content`
- explicit `entities` (files, services, technologies, decisions)

Use:

- `publish` for durable artifacts
- `promote` for upgrading important memories
- `knowledge_detail` when recall summaries are insufficient

## 6. Recommended Execution Pattern

Startup:

1. `list_scopes`
2. resolve scope
3. `session_begin`
4. `context`

During work:

1. `recall` before each new task
2. `remember` after meaningful changes
3. `skill_search` before building custom automation

Wrap-up:

1. optional `summarize(scope, topic)`
2. `publish`/`promote` important outcomes
3. `session_end(session_id)`

## Gotchas

- Do not proceed without successful `list_scopes`.
- Do not rely only on automatic hook snapshots for key decisions.
- Do not write ambiguous scope or entity values.
- Do not skip `session_end`.

## Workflow Checklist

- [ ] server reachable via `list_scopes`
- [ ] scope resolved and persisted (`.claude` or `.agents` base file)
- [ ] `session_begin` called
- [ ] `context` loaded
- [ ] `recall` before each non-trivial task
- [ ] explicit `remember` entries for decisions
- [ ] `session_end` called

## Validation Loop

For each major subtask:

1. Query `recall` with concrete terms.
2. Execute work.
3. Record key outcomes via `remember`.
4. Re-run `recall` to verify discoverability.

If discoverability is weak, refine summary/entity tags and repeat.

## Environment

- `POSTBRAIN_URL` default: `http://localhost:7433`
- `POSTBRAIN_TOKEN` required
- `POSTBRAIN_SCOPE` optional default
