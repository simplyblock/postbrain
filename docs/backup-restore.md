# Backup and Restore

Postbrain state is persisted in PostgreSQL. Back up the database before upgrades and regularly in production.

## What to back up

- Full Postbrain PostgreSQL database
- Runtime config values
- Kubernetes/infra secrets used for OAuth, TLS, or DNS automation

## Backup example (pg_dump)

```bash
pg_dump --format=custom --no-owner --no-privileges \
  --dbname "postgres://user:pass@host:5432/postbrain" \
  --file postbrain.backup
```

## Restore example (pg_restore)

```bash
pg_restore --clean --if-exists --no-owner --no-privileges \
  --dbname "postgres://user:pass@host:5432/postbrain" \
  postbrain.backup
```

## Validation after restore

1. Start server with restored DB.
2. Confirm UI loads and token auth works.
3. Run a simple write/read memory cycle.

## Operational recommendation

- Run scheduled backups.
- Test restore periodically in a non-production environment.
