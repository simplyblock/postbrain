-- name: CreateConsolidation :one
INSERT INTO consolidations (scope_id, source_ids, result_id, strategy, reason)
VALUES ($1,$2,$3,$4,$5)
RETURNING id, scope_id, source_ids, result_id, strategy, reason, created_at;
