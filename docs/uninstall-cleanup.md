# Uninstall and Cleanup

This guide covers safe removal options.

Decide first whether you want to remove only runtime components or also permanently delete stored data.

## Binary/package uninstall

- Remove installed binaries (`postbrain`, `postbrain-cli`) from PATH locations.
- Remove service unit/container definitions as needed.

## Helm uninstall

```bash
helm uninstall postbrain -n default
```

Then verify resources are removed:

```bash
kubectl -n default get deploy,svc,secret,configmap | rg postbrain
```

## Data retention decision

Before deleting storage/database:

1. decide whether data should be retained
2. create a final backup if needed

## Database cleanup (destructive)

Only run when permanent deletion is intended.

- drop application database, or
- delete only Postbrain schemas/tables according to your DB policy

If you are uncertain, keep DB data and remove only compute/runtime resources first.
