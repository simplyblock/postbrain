# Postbrain — UI and Interaction Design

## Purpose

This document captures architecture-level decisions for human-facing surfaces:
Web UI and TUI.

## Web UI

High-level goals:

- make scopes, artifacts, and token/access workflows discoverable
- expose operational controls without requiring raw API calls
- provide graph and retrieval inspection for debugging and trust

Design constraints:

- server-rendered baseline with progressive interactions where useful
- explicit auth gating for all UI actions
- no bypass of REST/authz invariants in UI handlers

Core UI capability areas:

- memories, knowledge, skills browsing
- scope/principal/membership/token administration
- promotion/review queues
- graph exploration and indexing controls

## TUI

High-level goals:

- lightweight terminal workflows for browsing and maintenance
- consistent read/write semantics with REST authorization model
- safe mutation flows with explicit confirmation for destructive actions

Design constraints:

- model-view-update style architecture
- strongly typed REST client integration
- keyboard-first interaction and clear screen ownership

## Interaction principles

- scope selection should be explicit before mutations
- visibility and lifecycle state should be surfaced clearly
- user-facing operations should preserve auditability

## Related documents

- `docs/webui-guide.md`
- `designs/DESIGN_API_AND_INTEGRATIONS.md`
- `designs/DESIGN_PERMISSIONS.md`
- `designs/TASKS.md`
