---
name: postbrain
description: Use this skill when Codex hooks are enabled and Postbrain should capture and retrieve task context.
version: 1
allowed-tools: Bash(postbrain-cli *)
---

# Postbrain — Codex (Hooks-Enabled)

Use this profile when repo-local hooks already run `postbrain-cli snapshot` and `postbrain-cli summarize-session`.

## Startup

1. Verify server reachability with `list_scopes`.
2. Resolve scope from `.agents/postbrain-base.md` first, then `README.md` or `AGENTS.md`.
3. Start a session: `session_begin(scope=...)`.
4. Load context: `context(scope=..., query=...)`.

When persisting scope defaults, use this canonical format in `.agents/postbrain-base.md`:

```md
---
postbrain_enabled: true
postbrain_scope: project:acme/api
updated_at: YYYY-MM-DD
---
```

## Defaults

- Never invent scope values. Scope must be `kind:external_id`.
- Default memory mode is `episodic` for tool/task progress and `semantic` for durable outcomes.
- Even with hooks enabled, still write explicit `remember` entries for decisions and constraints.

## Core Commands

- `recall(query, scope, layers, limit)` before each non-trivial task.
- `remember(content, scope, memory_type, summary, entities, source_ref)` after meaningful decisions.
- `publish(...)` / `promote(...)` for long-lived artifacts.
- `session_end(session_id)` before stopping.

## Gotchas

- Do not continue if `list_scopes` fails.
- Do not skip `session_end`; missing close events reduce session quality.
- Do not store weak entity tags; include files/services/technologies explicitly.

## Workflow Checklist

- [ ] `list_scopes` successful
- [ ] scope resolved from `.agents/postbrain-base.md` (or explicitly provided)
- [ ] `session_begin` called
- [ ] `context` loaded
- [ ] `recall` run before each new task
- [ ] explicit `remember` for decisions
- [ ] `session_end` called

## Validation Loop

After each major step:

1. Verify recall quality with a focused `recall` query.
2. If results are weak, add one higher-signal `remember` entry.
3. Re-run `recall` to confirm the memory is retrievable.

## Environment

- `POSTBRAIN_URL` (default `http://localhost:7433`)
- `POSTBRAIN_TOKEN` (required)
- `POSTBRAIN_SCOPE` (optional default)
