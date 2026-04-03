# Backup and Restore

Postbrain state is persisted in PostgreSQL. Back up the database before upgrades and regularly in production.

## What to back up

At minimum, capture:

- Full Postbrain PostgreSQL database
- Runtime config values (or Helm values files)
- Kubernetes/infra secrets used for OAuth, TLS, or DNS automation

Without config and secret backups, a database-only restore can still leave the service unusable.

## Backup example (pg_dump)

Use a consistent backup format you can restore quickly:

```bash
pg_dump --format=custom --no-owner --no-privileges \
  --dbname "postgres://user:pass@host:5432/postbrain" \
  --file postbrain.backup
```

## Restore example (pg_restore)

Restore into a controlled environment first whenever possible:

```bash
pg_restore --clean --if-exists --no-owner --no-privileges \
  --dbname "postgres://user:pass@host:5432/postbrain" \
  postbrain.backup
```

## Validation after restore

A restore is only complete when behavior is validated:

1. Start server with restored DB.
2. Confirm UI loads and token auth works.
3. Run a simple write/read memory cycle.

## Operational recommendation

- Run scheduled backups.
- Test restore periodically in a non-production environment.

Restore testing should be part of routine operations, not only incident response. The first real restore should not be
during an outage.
