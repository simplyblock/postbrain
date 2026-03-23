-- name: InsertStalenessFlag :one
INSERT INTO staleness_flags (artifact_id, signal, confidence, evidence, status)
VALUES ($1,$2,$3,$4,COALESCE($5,'open'))
RETURNING id, artifact_id, signal, confidence, evidence, status, flagged_at,
          reviewed_by, reviewed_at, review_note;

-- name: HasOpenStalenessFlag :one
SELECT EXISTS(
    SELECT 1 FROM staleness_flags
    WHERE artifact_id=$1 AND signal=$2 AND status='open'
);

-- name: UpdateStalenessFlag :exec
UPDATE staleness_flags
SET status=$2, reviewed_by=$3, review_note=$4, reviewed_at=now()
WHERE id=$1;

-- name: ListStalenessFlags :many
SELECT id, artifact_id, signal, confidence, evidence, status, flagged_at,
       reviewed_by, reviewed_at, review_note
FROM staleness_flags
WHERE ($1='' OR status=$1)
ORDER BY flagged_at DESC
LIMIT $2 OFFSET $3;
