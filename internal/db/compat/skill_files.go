package compat

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
)

// ListSkillFiles returns all supplementary files for a skill, ordered by relative_path.
func ListSkillFiles(ctx context.Context, pool *pgxpool.Pool, skillID uuid.UUID) ([]*db.SkillFile, error) {
	q := db.New(pool)
	return q.ListSkillFiles(ctx, skillID)
}

// UpsertSkillFile inserts or updates a supplementary file for a skill.
func UpsertSkillFile(ctx context.Context, pool *pgxpool.Pool, skillID uuid.UUID, relativePath, content string, isExecutable bool) (*db.SkillFile, error) {
	q := db.New(pool)
	return q.UpsertSkillFile(ctx, db.UpsertSkillFileParams{
		SkillID:      skillID,
		RelativePath: relativePath,
		Content:      content,
		IsExecutable: isExecutable,
	})
}

// GetSkillFile returns a single supplementary file by skill ID and relative path.
// Returns nil, nil if not found.
func GetSkillFile(ctx context.Context, pool *pgxpool.Pool, skillID uuid.UUID, relativePath string) (*db.SkillFile, error) {
	q := db.New(pool)
	f, err := q.GetSkillFile(ctx, db.GetSkillFileParams{
		SkillID:      skillID,
		RelativePath: relativePath,
	})
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return f, err
}

// DeleteSkillFile removes a single supplementary file by skill ID and relative path.
func DeleteSkillFile(ctx context.Context, pool *pgxpool.Pool, skillID uuid.UUID, relativePath string) error {
	q := db.New(pool)
	return q.DeleteSkillFile(ctx, db.DeleteSkillFileParams{
		SkillID:      skillID,
		RelativePath: relativePath,
	})
}

// DeleteAllSkillFiles removes all supplementary files for a skill.
func DeleteAllSkillFiles(ctx context.Context, pool *pgxpool.Pool, skillID uuid.UUID) error {
	q := db.New(pool)
	return q.DeleteAllSkillFiles(ctx, skillID)
}

// SnapshotSkillFiles copies the current supplementary files into skill_history_files
// at the given version number. Call before updating skill content.
func SnapshotSkillFiles(ctx context.Context, pool *pgxpool.Pool, skillID uuid.UUID, version int32) error {
	q := db.New(pool)
	return q.SnapshotSkillFiles(ctx, db.SnapshotSkillFilesParams{
		SkillID: skillID,
		Version: version,
	})
}

// ListSkillHistoryFiles returns the file snapshots for a skill at a specific version.
func ListSkillHistoryFiles(ctx context.Context, pool *pgxpool.Pool, skillID uuid.UUID, version int32) ([]*db.SkillHistoryFile, error) {
	q := db.New(pool)
	return q.ListSkillHistoryFiles(ctx, db.ListSkillHistoryFilesParams{
		SkillID: skillID,
		Version: version,
	})
}
