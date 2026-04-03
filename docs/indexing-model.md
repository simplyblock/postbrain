# Indexing Model

This page explains what Postbrain indexes, which records are created, and how those records are used during retrieval.

The goal of the indexing model is to make recall both accurate and explainable: semantic similarity alone is not enough
for real project memory, and pure keyword search misses conceptual links.

## Core indexed items

Postbrain indexes these primary item types:

- memories: short-lived iterative notes and outcomes
- knowledge artifacts: curated long-lived content
- chunks: artifact subdivisions used for embedding and retrieval granularity
- entities: graph nodes extracted or generated for linking concepts
- relations: graph edges connecting entities and artifacts/chunks

These item types work together. For example, a recall query can find a chunk via vector similarity, then improve ranking
using entity relations attached to that chunk or its artifact.

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

In practical terms, chunking helps with long documents where only one section answers the query, while `next_chunk`
preserves neighborhood context for downstream summarization or response generation.

## Entity and relation indexing

Entity graph indexing captures relationships between content and concepts.

Typical indexed entity categories include:

- artifact entities
- artifact chunk entities
- code/document concepts when available from extractors/enrichment

Relations connect those entities for graph-assisted recall.

Graph context is especially useful when query wording does not match stored wording exactly, but related entities still
connect the user query to the right content.

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

Scope filtering is applied as a hard boundary, not a soft preference. Only authorized scope data participates in final
results.

## Lifecycle and maintenance

Index data evolves through:

- initial writes (`remember`, `publish`, ingestion flows)
- re-embedding jobs
- summary/chunk backfill jobs
- staleness/maintenance jobs

Relevant controls are in `jobs.*` and `embedding.*` configuration.

This lifecycle means indexing is not a one-time operation. As models, content, and code evolve, background jobs keep
the index consistent and useful over time.

## End-to-end example

Given an uploaded design document:

1. Postbrain stores the artifact and splits it into chunks.
2. Chunks are embedded for semantic recall.
3. Chunk entities are linked to artifact entity (`chunk_of`) and to adjacent chunks (`next_chunk`).
4. A later query matches one chunk semantically and is re-ranked with graph/lexical context.
5. Only chunks in authorized scopes are returned.

## Why this matters

The indexing model balances:

- recall quality (chunked + semantic + lexical)
- explainability (entity/relation graph)
- security (strict scope filtering)
- maintainability (background refresh/backfill flows)
