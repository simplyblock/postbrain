# Go LSP (`gopls`) TCP Integration — Tasks

Date: 2026-04-02  
Status: In Progress  
Owner: Engineering

## Goal

Introduce an initial Go-only LSP resolver over TCP to improve callsite/callee resolution for code graph indexing, while keeping the existing import-aware and suffix fallback paths.

## Scope (Initial Slice)

- Language: Go (`.go`) only
- Transport: TCP between Postbrain and `gopls`
- Capability focus: resolve call targets for graph `calls` edges
- Fallback behavior: if LSP is unavailable or fails, continue indexing using existing resolver stages

## Tasks

- [x] Add resolver wiring with tests first:
  - [x] Extend `internal/codegraph/Resolver` to use optional LSP stage between import-aware and suffix fallback.
  - [x] Ensure non-LSP flows remain unchanged.
- [x] Implement Go TCP LSP resolver with tests first:
  - [x] Add `gopls` TCP client (`initialize`, `workspace/symbol` initial path, `shutdown`, `exit`).
  - [x] Add resolver implementation that maps LSP output to canonical symbol names.
  - [x] Add lifecycle cleanup (`Close`) and timeout handling.
- [x] Add indexer opt-in wiring with tests first:
  - [x] Add `IndexOptions` knob for Go LSP endpoint (TCP address).
  - [x] Start resolver once per index run, pass into per-file resolution, and close on completion.
  - [x] Degrade gracefully to existing resolver path if LSP cannot be used.
- [ ] Add integration-style test (guarded/skippable when `gopls` unavailable):
  - [ ] Tiny Go fixture repo with cross-file calls.
  - [ ] Assert at least one call edge resolved via LSP path.
- [ ] Add observability:
  - [ ] log one warning per run when LSP init fails
  - [ ] counters in index result/logs for LSP-assisted resolutions (initial minimal metric/log field)

## Acceptance Criteria

- TCP `gopls` resolver can be enabled for index runs without breaking existing behavior.
- When enabled and available, it contributes to call target resolution.
- When disabled/unavailable, indexing still completes using existing pipeline.
- `go test ./...`, `make test-integration`, `make lint` pass.
