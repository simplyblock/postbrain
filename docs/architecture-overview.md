# Architecture Overview

This page explains how Postbrain works at a technical level, in product terms.

## Main parts

Postbrain has three core runtime parts:

- API layer: REST + MCP endpoints used by apps, agents, and automation
- Storage layer: PostgreSQL with schema migrations and indexed retrieval
- Processing layer: embedding, summarization, enrichment, and background maintenance jobs

## Core data concepts

- principals: identities (users, agents, teams)
- scopes: visibility and ownership boundaries
- memories: iterative context units
- knowledge artifacts: long-lived curated documents
- entities and relations: graph links between concepts, files, and artifacts
- skills: reusable instruction payloads for agent tools

## Write path (simplified)

1. client sends a write request (`remember`, `publish`, etc.)
2. Postbrain validates authentication and scope access
3. content is embedded/analyzed where applicable
4. data is stored and linked to relevant entities
5. optional enrichment runs without blocking primary writes

## Retrieval path (simplified)

1. client sends a query (`recall` or `context`)
2. Postbrain resolves authorized scopes
3. retrieval combines:
    - semantic vector similarity
    - text ranking (FTS)
    - trigram fuzzy matching
    - graph/entity relation context
4. results are merged, scored, and returned

## Why this design works well

- better recall quality than single-method search
- clear scope boundaries for multi-team usage
- durable promotion path from short-lived memory to long-lived knowledge
- resilient writes (best-effort enrichment does not block core operations)

For a detailed breakdown of indexed records (artifacts, chunks, entities, relations), see
[Indexing Model](./indexing-model.md).

## Deployment model

Postbrain is typically deployed as:

- one API service
- one PostgreSQL database
- optional embedding model providers (local or cloud)

This keeps operations simple while still supporting large project histories.
