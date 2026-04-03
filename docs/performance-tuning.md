# Performance and Cost Tuning

This guide covers practical tuning without changing architecture.

## Embedding settings

Tune in `embedding.*`:

- `batch_size`: larger batches improve throughput, increase memory use.
- `request_timeout`: increase for slower backends.
- backend/model choice drives quality/cost profile.

## Database settings

Tune in `database.*`:

- `max_open`
- `max_idle`
- `connect_timeout`

Choose values based on DB capacity and concurrency.

## Job controls

Use `jobs.*` toggles to enable only required background workloads.

Examples:

- disable heavy backfills outside migration windows
- pause optional jobs during incident response

## Practical approach

1. Baseline latency/error metrics.
2. Change one knob at a time.
3. Compare cost/performance after each change.
