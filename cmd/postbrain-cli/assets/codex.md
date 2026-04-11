---
name: postbrain
description: Postbrain Codex persistent memory and knowledge operating policy
version: 1
allowed-tools: Bash(postbrain-cli *)
---

# Postbrain — Persistent Memory & Knowledge for Codex

This skill defines the mandatory operating pattern for Postbrain usage in Codex.

## 1. Server Gate

Before anything else, run `list_scopes`.

If it fails, stop and report:

> Postbrain MCP server is not reachable. Ensure `POSTBRAIN_URL` (default `http://localhost:7433`) and `POSTBRAIN_TOKEN` are valid.

## 2. Scope Resolution

Scope format is always `kind:external_id` (example: `project:acme/api`).

Resolve scope in this order:

1. `.agents/postbrain-base.md`
2. `README.md`
3. `AGENTS.md`
4. local docs mentioning `POSTBRAIN_SCOPE`

If still missing, ask user and persist to `.agents/postbrain-base.md`.
Never invent scope values.

## 3. Session Bootstrap

1. `session_begin(scope=...)`
2. `context(scope=..., query=<current task>, max_tokens=4000)`
3. `recall(query=<task>, scope=...)` before each non-trivial task

Use `knowledge_detail(artifact_id)` when recall summary is insufficient.

## 4. Memory Write Rules

Use `remember` with precise structure:

- `summary`: short high-signal statement
- `content`: details (what changed, why, constraints)
- `entities`: explicit tags (`technology`, `service`, `file`, `decision`, `concept`, `person`, `pr`)

Memory types:

- `working`: temporary notes, optional TTL
- `episodic`: task/tool progress
- `procedural`: repeatable steps/runbooks
- `semantic`: stable facts and architecture

## 5. Artifact & Skill Rules

- Use `publish` for durable knowledge.
- Use `promote` when a memory should become an artifact.
- Use `endorse` for quality signaling.
- Use `skill_search` before reinventing automation.
- Use `skill_install`/`skill_invoke` when relevant.

## 6. Recommended Execution Pattern

Startup:

1. `list_scopes`
2. resolve scope
3. `session_begin`
4. `context`

During work:

1. `recall` before each task
2. `remember` after meaningful steps/decisions
3. `knowledge_detail` when needed
4. `skill_search` for reusable workflows

Wrap-up:

1. optional `summarize(scope, topic)`
2. `publish`/`promote` important outcomes
3. `session_end(session_id)`

## Gotchas

- Do not proceed after failed reachability checks.
- Do not leave scope implicit or guessed.
- Do not write low-signal memories without entities/summary.
- Do not skip `session_end`; it harms session traceability.

## Workflow Checklist

- [ ] `list_scopes` succeeded
- [ ] scope found or explicitly provided
- [ ] scope persisted in `.agents/postbrain-base.md` if newly set
- [ ] `session_begin` called
- [ ] `context` loaded
- [ ] `recall` before each non-trivial task
- [ ] high-value `remember` entries written
- [ ] `session_end` called

## Validation Loop

Repeat this cycle for each substantial subtask:

1. Retrieve: run `recall` with concrete task query.
2. Execute: perform work using retrieved context.
3. Capture: write `remember` for new decisions/constraints.
4. Verify: re-run targeted `recall` to confirm retrievability.

If retrieval quality is weak, improve entity tags and summary granularity, then repeat.

## Environment

- `POSTBRAIN_URL` default: `http://localhost:7433`
- `POSTBRAIN_TOKEN` required
- `POSTBRAIN_SCOPE` optional default
