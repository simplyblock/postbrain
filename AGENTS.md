# Agent Rules and Guard Rails

This file defines the mandatory rules and constraints for all AI coding agents
working in this repository (Claude Code, OpenAI Codex, GitHub Copilot, and
any other agent). Rules here are not suggestions — they are required behaviour.

---

## Always Do

### Design Intake

Before implementation, use `docs/index.md` as the **starting point** for design-level context.

- Read `docs/index.md` first to identify which `designs/DESIGN*.md` and `designs/TASKS*.md` files apply to your change.
- Treat `designs/DESIGN.md` as the high-level architecture overview; use focused design files for subsystem specifics.
- Read every applicable design/task file in full before modifying related code.
- If scope is unclear or spans multiple subsystems, default to reading `designs/DESIGN.md` in full.

### Test-Driven Development

All development follows **strict TDD** — no exceptions:

1. **Red** — write a failing test that specifies the desired behaviour.
   A test that does not yet compile (missing type, function, or method) counts
   as red. Confirm it fails before proceeding.
2. **Green** — write the minimum implementation to make the test pass.
   Do not write more code than the test requires.
3. **Refactor** — clean up implementation and tests, keeping everything green.

Additional rules:
- Never write implementation code before the test exists.
- Tests drive API design — write the test as a caller would use the code.
- Bug fixes: write a failing regression test first, then fix the code.
- New features: write the full test suite first, then implement.
- Tests live alongside the code they test (`foo_test.go` next to `foo.go`).
- Unit tests: no external dependencies, tagged with no build tag, run with
  `go test ./...` (`make test`).
- Integration tests: require Postgres, tagged `//go:build integration`, file
  names end in `_integration_test.go`, use `testhelper.NewTestPool`, run with
  `make test-integration`.
- Use `embedding.FakeEmbedder` for any test that needs an embedder.
- Use `noopXxx` base structs for interface fakes (see `noopLifecycleDB` in
  `internal/knowledge/lifecycle_test.go` as the reference implementation).
- Table-driven tests for functions with multiple input variants.
- Every test helper must call `t.Helper()` and `t.Fatal`/`t.Error` directly.

### Test Suite
- Run the **full test suite** before staging and committing any changes:
  ```
  go test ./...
  ```
- Newly written tests must pass. No exceptions.
- If an existing test breaks as a side effect, fix it before committing —
  do not work around it or skip it.

### Formatting and Linting
- Run the code formatter before every commit:
  ```
  gofmt -w .
  ```
- Code that does not pass `gofmt` must not be committed.
- Run the linter before every commit:
  ```
  make lint
  ```
- Linting **must pass**. Do not commit code with lint errors. Fix all issues
  before committing — do not suppress warnings unless there is a documented
  reason.

### Committing
- Commit after each completed task or prompt iteration where changes were made.
- `git add` and `git commit` are always permitted in this repository without
  asking for confirmation.
- Do **not** amend existing commits unless explicitly asked.
- Commit messages must include:
  - A short summary line (≤ 72 characters).
  - A meaningful body explaining *what* changed and *why*.
  - A `Co-authored-by:` trailer for every agent-created commit, using the
    current agent's own identity (name + no-reply email):
    ```
    Co-authored-by: <Agent Name> <noreply@provider.example>
    ```

### Task Tracking
- Update `designs/TASKS.md` **before** staging and committing. Record what was done,
  mark completed tasks, and add any newly discovered tasks.

### In-Code Markers
- Add `TODO` and `FIXME` comments at source locations that will need future
  attention — incomplete logic, known limitations, deferred clean-up:
  ```go
  // TODO(task-N): replace with proper retry logic once backoff package is added
  // FIXME: this panics if embedding returns an empty vector
  ```
- Do not silently leave code in a broken or incomplete state without a marker.

### Change Scope
- Make small, focused changes. One logical change per commit.
- Preserve the project's minimalism — only add what the current task requires.

### Search
- Prefer `rg` (ripgrep) for searching across the codebase:
  ```
  rg "pattern" --type go
  ```

### Design Changes
- Update `designs/DESIGN.md` **only if absolutely necessary**.
- You **must ask** before making any change to `designs/DESIGN.md`, and provide a clear
  explanation of why the design must be adjusted.

---

## Never Do

### Dependencies
- Do not introduce large frameworks or heavy dependencies unless they are
  explicitly approved in `designs/DESIGN.md` or by the user.
- New dependencies must be the minimum necessary to fulfil the task.

### Documentation Files
- Do not create additional documentation files (markdown, READMEs, wikis)
  unless explicitly asked.

### Existing Behaviour
- Do not change existing behaviour without explicit consent.
- Do not "improve", "clean up", or refactor code you were not asked to change.
  If you notice something that should be cleaned up, add a `TODO` comment and
  move on.
- Do not break existing code flow. If a change risks breaking something, flag
  it and ask.

---

## Code Style

### Go
- Follow standard Go style as enforced by `gofmt`.
- Avoid unnecessary abstractions and interfaces. Only introduce an abstraction
  when there are two or more concrete implementations today, not hypothetically.
- Prefer clarity over cleverness. If a simpler approach exists, use it.
- Error handling: always check and handle errors explicitly. Do not use `_` to
  discard errors in production code paths.
- Package names: short, lowercase, no underscores.
- Exported identifiers must have a doc comment only when their purpose is not
  immediately obvious from the name alone.

---

## Git Workflow

| Rule | Detail |
|------|--------|
| Commit frequency | After each completed prompt iteration that produced changes |
| Amend | Never, unless explicitly requested |
| `git add` / `git commit` | Always permitted without asking |
| Commit message summary | ≤ 72 characters, imperative mood ("Add X", "Fix Y") |
| Commit message body | Required — explain what and why, not just what |
| `Co-authored-by` trailer | Required on every agent commit |
| Pre-commit checklist | 1. `go test ./...` passes · 2. `gofmt -w .` applied · 3. `make lint` passes · 4. `designs/TASKS.md` updated |

---

## Pre-Commit Checklist

Before every `git commit`, verify all of the following:

- [ ] `go test ./...` — full test suite passes, including newly written tests
- [ ] `gofmt -w .` — formatter applied, no outstanding diffs
- [ ] `make lint` — linter passes with zero errors
- [ ] `designs/TASKS.md` — updated to reflect completed and newly discovered work
- [ ] Changes are focused — no unrelated modifications staged
- [ ] `designs/DESIGN.md` — not modified unless approved and necessary

<!-- postbrain-config -->
## Postbrain

The `.codex/skills/postbrain/SKILL.md` skill is active for this project.

```
POSTBRAIN_URL=http://localhost:7433
# POSTBRAIN_SCOPE=project:your-org/your-repo   <- set this to skip the scope prompt
```
