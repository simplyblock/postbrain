# Performance and Cost Tuning

This guide covers practical tuning without changing architecture. Focus on measurement-first changes so you can tell
which setting actually improved behavior.

## Embedding settings

Tune in `embedding.*`:

- `batch_size`: larger batches improve throughput, increase memory use.
- `request_timeout`: increase for slower backends.
- backend/model choice drives quality/cost profile.

In practice, model/backend choice has the biggest quality/cost impact, while `batch_size` and timeout settings control
throughput stability.

## Database settings

Tune in `database.*`:

- `max_open`
- `max_idle`
- `connect_timeout`

Choose values based on DB capacity and concurrency.
Too-high pool values can degrade DB performance instead of improving throughput.

## Job controls

Use `jobs.*` toggles to enable only required background workloads.

Examples:

- disable heavy backfills outside migration windows
- pause optional jobs during incident response

## Practical approach

Treat tuning as an iterative workflow:

1. Baseline latency/error metrics.
2. Change one knob at a time.
3. Compare cost/performance after each change.

If you change multiple controls at once, it becomes hard to identify the actual cause of improvements or regressions.
