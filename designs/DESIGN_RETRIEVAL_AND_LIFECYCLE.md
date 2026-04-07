# Postbrain — Retrieval and Lifecycle Design

## Purpose

This document describes how Postbrain retrieves information across layers and
how artifacts move through review, visibility, and synthesis lifecycles.

## Retrieval strategy

Retrieval is hybrid and multi-stage:

1. candidate gathering per layer (vector, lexical, metadata)
2. per-layer scoring and normalization
3. cross-layer merge and rerank
4. deduplication/suppression rules

Key principle:

- layer blending is allowed, but authz and scope filtering happen first

## Layer-aware retrieval behavior

- memory emphasizes recency, importance, and semantic relevance
- knowledge emphasizes publication status, visibility, and endorsements
- skills emphasize compatibility, status, and relevance to task intent

## Scope and visibility behavior

- memory retrieval may fan out through configured scope ancestry
- knowledge/skills visibility is explicit (`private` -> `company`)
- sharing grants can expose specific artifacts without changing ownership

For the exact permission model, see `designs/DESIGN_PERMISSIONS.md`.

## Promotion workflow

Primary path:

- memory is captured in scope
- promotion request proposes target scope and visibility
- review/approval creates or updates knowledge artifact
- procedural knowledge can be promoted into reusable skill artifacts

## Knowledge lifecycle

- `draft` -> `in_review` -> `published` -> `deprecated`
- endorsement thresholds control auto-publish behavior from review
- version history and provenance must remain auditable

## Topic synthesis

Digest workflow:

- synthesize multiple published artifacts into a digest artifact
- once published, digest may suppress sources in default retrieval views
- sources remain directly retrievable when explicitly requested

## Staleness and maintenance signals

Representative staleness signals include:

- changed source context
- contradictions from newer evidence
- low access + age decay

These signals influence review/maintenance actions, not hard deletion by
default.

## Related documents

- `designs/DESIGN_DATA_MODEL.md`
- `designs/DESIGN_API_AND_INTEGRATIONS.md`
- `designs/DESIGN_CODE_GRAPH.md`
- `designs/TASKS.md`
