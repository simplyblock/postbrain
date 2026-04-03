# Upgrade Guide

This guide covers safe upgrades for Postbrain deployments and explains why each step matters. The short version is:
always back up first, change one layer at a time, and run a smoke test before calling the upgrade complete.

## Before upgrading

Before upgrading, make sure you can answer three questions:

1. What changed in the target release?
2. Can we restore data if something fails?
3. How do we verify the service is healthy afterward?

Use this checklist:

1. Read release notes for your target version.
2. Back up your database.
3. Export your current runtime config (or Helm values).
4. Verify PostgreSQL extensions are available:
   `vector`, `pg_trgm`, `uuid-ossp`, `pg_cron`, `pg_partman`.

## Standard upgrade flow

This flow works for binary, container, and Helm-based deployments:

1. Stop old server process (or roll a new deployment revision).
2. Deploy new binary/image/chart version.
3. Start server with `database.auto_migrate=true` (or run migration step explicitly).
4. Verify health endpoints and basic read/write behavior.

## Kubernetes/Helm upgrade

For Helm installations, a normal in-place upgrade looks like this:

```bash
helm upgrade postbrain ./deploy/helm/postbrain -n default -f values.yaml
kubectl -n default rollout status deploy/postbrain
```

If you use Gateway API/cert-manager, also verify those resources after rollout.

## Binary upgrade

For direct process deployments:

1. Replace `postbrain` binary.
2. Keep config file path stable.
3. Restart service.

## Validate after upgrade

Run a small but meaningful smoke test before ending the maintenance window:

1. `postbrain version`
2. token-authenticated API call
3. one `remember` write and one `recall` read
4. UI load (`/ui`)

This catches most real-world upgrade issues: auth breakage, DB/migration mismatches, and retrieval regressions.

## Rollback

If startup or migration validation fails, roll back in this order:

1. Roll back application version first.
2. Restore DB from backup only if required.
3. Re-run with tested previous version and confirm health.

Avoid restoring data unless you have to. Many issues are application/config-level and can be fixed by reverting the
runtime version only.
