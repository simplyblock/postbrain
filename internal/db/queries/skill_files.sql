-- name: ListSkillFiles :many
SELECT id, skill_id, relative_path, content, is_executable, created_at, updated_at
FROM skill_files
WHERE skill_id = $1
ORDER BY relative_path;

-- name: GetSkillFile :one
SELECT id, skill_id, relative_path, content, is_executable, created_at, updated_at
FROM skill_files
WHERE skill_id = $1 AND relative_path = $2;

-- name: UpsertSkillFile :one
INSERT INTO skill_files (skill_id, relative_path, content, is_executable)
VALUES ($1, $2, $3, $4)
ON CONFLICT (skill_id, relative_path)
DO UPDATE SET
    content       = EXCLUDED.content,
    is_executable = EXCLUDED.is_executable,
    updated_at    = now()
RETURNING id, skill_id, relative_path, content, is_executable, created_at, updated_at;

-- name: DeleteSkillFile :exec
DELETE FROM skill_files
WHERE skill_id = $1 AND relative_path = $2;

-- name: DeleteAllSkillFiles :exec
DELETE FROM skill_files
WHERE skill_id = $1;

-- name: SnapshotSkillFiles :exec
INSERT INTO skill_history_files (skill_id, version, relative_path, content, is_executable)
SELECT sf.skill_id, $2, sf.relative_path, sf.content, sf.is_executable
FROM skill_files sf
WHERE sf.skill_id = $1;

-- name: ListSkillHistoryFiles :many
SELECT id, skill_id, version, relative_path, content, is_executable, created_at
FROM skill_history_files
WHERE skill_id = $1 AND version = $2
ORDER BY relative_path;
