# Upgrade Guide

This guide covers safe upgrades for Postbrain server deployments.

## Before upgrading

1. Read release notes for your target version.
2. Back up your database.
3. Export your current runtime config.
4. Verify PostgreSQL extensions are available (`vector`, `pg_trgm`, `uuid-ossp`, `pg_cron`, `pg_partman`).

## Standard upgrade flow

1. Stop old server process (or roll a new deployment revision).
2. Deploy new binary/image/chart version.
3. Start server with `database.auto_migrate=true` (or run migration step explicitly).
4. Verify health endpoints and basic read/write behavior.

## Kubernetes/Helm upgrade

```bash
helm upgrade postbrain ./deploy/helm/postbrain -n default -f values.yaml
kubectl -n default rollout status deploy/postbrain
```

## Binary upgrade

1. Replace `postbrain` binary.
2. Keep config file path stable.
3. Restart service.

## Validate after upgrade

Run a quick smoke test:

1. `postbrain version`
2. token-authenticated API call
3. one `remember` write and one `recall` read
4. UI load (`/ui`)

## Rollback

If startup or migration validation fails:

1. Roll back application version first.
2. Restore DB from backup only if required.
3. Re-run with tested previous version and confirm health.
