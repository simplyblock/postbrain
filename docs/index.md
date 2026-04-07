# Postbrain Docs Index for Design and Task Files

This page is the starting point for design-level context before code changes.
Use it to decide which `designs/` files you must read in full for a task.

## How to use this index

1. Locate the area you are changing.
2. Read the listed design file summary and "Read in full when..." rule.
3. Open the paired tasks file to confirm current status and pending work.
4. If your change crosses multiple areas (for example OAuth + permissions), read all related design/task pairs.

## Design and task map

| Design file | Paired task file | What it covers | Read in full when... |
|---|---|---|---|
| `designs/DESIGN.md` | `designs/TASKS.md` | Core system architecture: three-layer model (memory/knowledge/skills), storage model, API surfaces, lifecycle/workflow expectations, and repository-wide implementation plan. | You are changing core behavior, DB/API contracts, scope semantics, or anything outside a narrowly scoped subsystem. |
| `designs/DESIGN_PERMISSIONS.md` | `designs/TASKS_PERMISSIONS.md` | Full authorization model: resource:operation permissions, role derivation, scope inheritance, token downscoping, and cross-surface authz enforcement. | You touch auth middleware, token handling, scope access, role logic, REST/MCP/WebUI authorization checks, or permission wording in docs. |
| `designs/DESIGN_OAUTH.md` | `designs/TASKS_OAUTH.md` | OAuth design for two modes: social login (Postbrain as OAuth client) and MCP/OAuth server flows (Authorization Code + PKCE), including schema/config requirements. | You change login, session/cookie auth, OAuth endpoints, provider config, token exchange, or OAuth-related DB objects. |
| `designs/DESIGN_CODE_GRAPH.md` | `designs/TASKS_CODE_GRAPH.md` | Code graph extraction/indexing design: entity/relation model, extraction pipeline, resolution strategies (heuristic/import-aware/LSP), repo indexing, and graph retrieval integration. | You touch `internal/codegraph`, graph relations/traversal, repo sync/indexing behavior, or graph-augmented recall. |

## Additional task trackers without a dedicated design doc

| Task file | Purpose | Read in full when... |
|---|---|---|
| `designs/TASKS_EMBEDDING_UPDATE.md` | Embedding architecture migration plan (model-scoped tables, provider/model registration, re-embed/backfill behavior, cutover decisions). | You change embedding storage, model registration, embedding jobs, or recall behavior across model versions/providers. |
| `designs/TASKS_LSP_GO.md` | Focused plan for Go `gopls` TCP integration in code graph resolution and indexer wiring. | You touch Go LSP resolver behavior, indexer LSP options, or observability around LSP-assisted resolution. |

## Fast routing hints

- Working in `internal/authz`, `internal/auth`, `internal/api/scopeauth`, REST/MCP permission middleware: start with `DESIGN_PERMISSIONS.md`.
- Working in `internal/oauth` or UI login/session/auth callbacks: start with `DESIGN_OAUTH.md`.
- Working in `internal/codegraph`, `internal/graph`, scope repo sync/index endpoints: start with `DESIGN_CODE_GRAPH.md`.
- Unsure or touching multiple layers: start with `DESIGN.md`, then drill down using this index.
