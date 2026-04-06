# Introduction to Postbrain

## Why this exists

AI assistants are fast, but they are still session-bound in most real workflows. Their built-in memory is usually tied
to a specific user/account/device/agent context, which means it does not naturally propagate across teammates, service
accounts, or organization scopes. Teams repeatedly lose context between tasks, re-discover the same decisions, and
rebuild prompts that should already exist as shared operational knowledge. This creates friction, inconsistent outcomes,
and unnecessary repetition.

Postbrain is the memory and knowledge layer for those workflows. It keeps useful context durable across sessions and
across agents, so work does not reset every time a chat window closes.

## What Postbrain is

Postbrain stores and retrieves three kinds of context:

1. `memory`: quick, high-frequency observations captured while work is happening
2. `knowledge`: durable artifacts meant to be reviewed, reused, and shared
3. `skills`: reusable instruction assets that can be installed into agent environments

This model supports both speed and quality. You can capture raw working notes immediately, then promote stable outcomes
into long-lived knowledge once they are ready.

## How memory and knowledge work

In a typical workflow, agents write memories continuously during task execution. Before starting new tasks, they recall
relevant context from prior work in the same scope. When a pattern, decision, or implementation detail becomes stable,
it is promoted into a knowledge artifact so future sessions can consume a cleaner, curated version.

The result is a practical lifecycle: capture fast, refine later, and reuse confidently.

## How retrieval works

Postbrain uses hybrid retrieval instead of relying on a single search strategy. Semantic vector search handles intent
and meaning; full-text search handles precise keyword and code-symbol lookups; trigram matching helps with fuzzy
queries; and graph traversal adds connected context (for example related artifacts, entities, and chunk relationships).

Because these strategies are combined and scope-filtered, retrieval is both more accurate and safer for multi-team
usage.

## Access and scope boundaries

All content is tied to scopes and principals. This means memory and knowledge are not only searchable, but also
constrained by authorization rules. A caller can only retrieve what its token and effective principal permissions allow
for the requested scope and resource operation.

This is essential in real organizations, where project, team, and company context must be shared intentionally rather
than globally.

## Who should use Postbrain

Postbrain is designed for teams that use coding agents or internal AI assistants as part of daily engineering work and
want reliable long-term memory without sacrificing access control. It is especially useful when multiple contributors
work on the same repositories, standards, and systems over time.

If you are setting up Postbrain for the first time, continue with [Getting Started](./getting-started.md).
