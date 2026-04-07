# Postbrain — Operations and Delivery Design

## Purpose

This document defines implementation and operational design constraints:
stack choices, deployment model, and runtime maintenance expectations.

## Implementation stack

Primary components:

- Go server and CLI
- PostgreSQL as single source of truth
- MCP + REST surfaces
- background jobs for data quality and maintenance

## Repository shape

Expected major areas:

- API handlers and middleware
- domain services (memory/knowledge/skills/retrieval)
- auth/authz
- DB migrations and query layer
- jobs and operational support
- UI surfaces and static assets

## Background jobs

Representative job families:

- memory consolidation and contradiction detection
- re-embed/backfill and model-transition jobs
- aging/TTL and staleness maintenance
- repository/index synchronization jobs where enabled

## Deployment model

- local/self-hosted via Docker Compose for development
- production deployment with hardened PostgreSQL and observability
- TLS, secret management, and token hygiene are baseline requirements

## Data and operational safeguards

- migrations must be safe and repeatable
- token handling never stores raw tokens
- service startup should fail fast on schema/model incompatibilities
- retention and partition policies must be explicit for high-volume tables

## Change management

- Prefer additive, backwards-compatible changes.
- Document any behaviorally breaking change in task/design updates.
- Keep operational toggles explicit in config and avoid hidden defaults.

## Related documents

- `docs/operations.md`
- `docs/production-hardening.md`
- `docs/monitoring-alerting.md`
- `docs/troubleshooting-playbook.md`
- `designs/TASKS.md`
