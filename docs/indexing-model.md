# Indexing Model

This page explains what Postbrain indexes, which records are created, and how those records are used during retrieval.

## Core indexed items

Postbrain indexes these primary item types:

- memories: short-lived iterative notes and outcomes
- knowledge artifacts: curated long-lived content
- chunks: artifact subdivisions used for embedding and retrieval granularity
- entities: graph nodes extracted or generated for linking concepts
- relations: graph edges connecting entities and artifacts/chunks

## Artifact and chunk indexing

When an artifact is written/published, Postbrain can split content into chunks for retrieval quality.

For chunked artifacts, indexing includes:

- artifact record
- chunk records (ordered sequence)
- embeddings for retrieval
- graph links:
  - `chunk_of` (chunk -> artifact)
  - `next_chunk` (chunk N -> chunk N+1)

This allows both semantic recall and structural traversal across a document.

## Entity and relation indexing

Entity graph indexing captures relationships between content and concepts.

Typical indexed entity categories include:

- artifact entities
- artifact chunk entities
- code/document concepts when available from extractors/enrichment

Relations connect those entities for graph-assisted recall.

## Scope-aware indexing

All indexed items are attached to scopes and principals according to authorization rules.

This ensures retrieval is constrained to authorized scope sets and prevents cross-scope leakage.

## Retrieval pipeline (how indexed items are used)

Retrieval combines multiple signals:

- vector similarity (semantic matches)
- full-text search (keyword-heavy queries)
- trigram similarity (fuzzy matching)
- graph/entity context (relation-aware expansion/scoring)

Results are merged and ranked after scope authorization.

## Lifecycle and maintenance

Index data evolves through:

- initial writes (`remember`, `publish`, ingestion flows)
- re-embedding jobs
- summary/chunk backfill jobs
- staleness/maintenance jobs

Relevant controls are in `jobs.*` and `embedding.*` configuration.

## Why this matters

The indexing model balances:

- recall quality (chunked + semantic + lexical)
- explainability (entity/relation graph)
- security (strict scope filtering)
- maintainability (background refresh/backfill flows)
