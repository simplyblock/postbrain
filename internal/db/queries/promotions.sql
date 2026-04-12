-- name: CreatePromotionRequest :one
INSERT INTO promotion_requests
(memory_id, requested_by, target_scope_id, target_visibility,
 proposed_title, proposed_collection_id)
VALUES ($1,$2,$3,$4,$5,$6)
RETURNING id, memory_id, requested_by, target_scope_id, target_visibility,
    proposed_title, proposed_collection_id, status, reviewer_id, review_note,
    reviewed_at, result_artifact_id, created_at;

-- name: GetPromotionRequest :one
SELECT id, memory_id, requested_by, target_scope_id, target_visibility,
    proposed_title, proposed_collection_id, status, reviewer_id, review_note,
    reviewed_at, result_artifact_id, created_at
FROM promotion_requests WHERE id=$1;

-- name: ListPendingPromotions :many
SELECT id, memory_id, requested_by, target_scope_id, target_visibility,
    proposed_title, proposed_collection_id, status, reviewer_id, review_note,
    reviewed_at, result_artifact_id, created_at
FROM promotion_requests
WHERE status='pending' AND target_scope_id=$1
ORDER BY created_at;

-- name: UpdatePromotionRequest :exec
UPDATE promotion_requests
SET status=$2, reviewer_id=$3, review_note=$4, reviewed_at=now(), result_artifact_id=$5
WHERE id=$1;

-- name: GetStalePromotionRequests :many
SELECT id, memory_id, target_scope_id, created_at
FROM promotion_requests
WHERE status = 'pending' AND created_at < now() - interval '24 hours'
ORDER BY created_at;
