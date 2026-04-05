# Embedding Model Operations Runbook

This runbook covers how to operate embedding models in Postbrain after the
model-table rework:

- register new models
- activate models per content type
- run re-embedding
- rollback safely
- manually reset failed embedding jobs

Use these procedures for production and staging.

## 1) Register a new embedding model

Registering a model creates or reuses an `embedding_models` entry, provisions a
per-model storage table, and seeds `embedding_index` pending rows for existing
objects.

```bash
postbrain-cli embedding-model register \
  --config config.yaml \
  --slug openai-text-3-small-v1 \
  --provider openai \
  --service-url https://api.openai.com/v1 \
  --provider-model text-embedding-3-small \
  --dimensions 1536 \
  --content-type text \
  --activate
```

For code embeddings:

```bash
postbrain-cli embedding-model register \
  --config config.yaml \
  --slug local-code-nomic-v1 \
  --provider ollama \
  --service-url http://localhost:11434 \
  --provider-model nomic-embed-text \
  --dimensions 768 \
  --content-type code \
  --activate
```

## 2) List and validate model state

```bash
postbrain-cli embedding-model list --config config.yaml
```

Check:

- `active=true` for exactly one model per `content_type`
- `ready=true` for active models
- `table_name` is present (`embeddings_model_<uuid_no_dashes>`)

Optional SQL check:

```sql
SELECT slug, content_type, is_active, is_ready, table_name
FROM embedding_models
ORDER BY content_type, slug;
```

## 3) Activate a model

Activation is independent from registration and applies per content type:

```bash
postbrain-cli embedding-model activate \
  --config config.yaml \
  --slug openai-text-3-small-v1 \
  --content-type text
```

```bash
postbrain-cli embedding-model activate \
  --config config.yaml \
  --slug local-code-nomic-v1 \
  --content-type code
```

## 4) Re-embedding behavior

Re-embedding now runs off `embedding_index`:

- jobs process rows with `status='pending'` for the active model
- success sets `status='ready'`, `retry_count=0`, `last_error=NULL`
- failures increment `retry_count` and store `last_error`
- after max retries, rows become `status='failed'`

Operational checks:

```sql
SELECT model_id, object_type, status, count(*) AS rows
FROM embedding_index
GROUP BY model_id, object_type, status
ORDER BY model_id, object_type, status;
```

```sql
SELECT object_type, object_id, model_id, retry_count, last_error, updated_at
FROM embedding_index
WHERE status = 'failed'
ORDER BY updated_at DESC
LIMIT 100;
```

## 5) Manual retry for failed rows

After fixing provider/network/model issues, reset failed rows to pending:

```sql
UPDATE embedding_index
SET status = 'pending',
    retry_count = 0,
    last_error = NULL,
    updated_at = now()
WHERE status = 'failed'
  AND model_id = '<MODEL_UUID>'::uuid;
```

You can also target a specific object:

```sql
UPDATE embedding_index
SET status = 'pending',
    retry_count = 0,
    last_error = NULL,
    updated_at = now()
WHERE object_type = 'memory'
  AND object_id = '<OBJECT_UUID>'::uuid
  AND model_id = '<MODEL_UUID>'::uuid;
```

## 6) Rollback procedure

If a newly activated model causes regressions:

1. Reactivate the previous known-good model (`embedding-model activate`).
2. Verify recall behavior in one project scope.
3. Keep failed rows for diagnostics, or reset only after root-cause fix.
4. Keep old model tables until migration cleanup is explicitly approved.

Rollback command:

```bash
postbrain-cli embedding-model activate \
  --config config.yaml \
  --slug <previous-model-slug> \
  --content-type text
```

## 7) Post-change acceptance checks

Run after register/activate/rollback operations:

1. `postbrain-cli embedding-model list` shows expected active model.
2. `embedding_index` has no unexpected growth in `failed`.
3. Query playground recall returns expected results for a known scope.
4. Logs show no repeating embedding/reembed errors.
