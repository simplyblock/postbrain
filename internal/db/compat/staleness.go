package compat

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/simplyblock/postbrain/internal/db"
)

// InsertStalenessFlag inserts a staleness_flags row.
func InsertStalenessFlag(ctx context.Context, pool *pgxpool.Pool, f *db.StalenessFlag) (*db.StalenessFlag, error) {
	if f.Evidence == nil {
		f.Evidence = []byte("{}")
	}
	q := db.New(pool)
	var statusPtr *string
	if f.Status != "" {
		statusPtr = &f.Status
	}
	result, err := q.InsertStalenessFlag(ctx, db.InsertStalenessFlagParams{
		ArtifactID: f.ArtifactID,
		Signal:     f.Signal,
		Confidence: f.Confidence,
		Evidence:   f.Evidence,
		Column5:    statusPtr,
	})
	if err != nil {
		return nil, fmt.Errorf("db: insert staleness flag: %w", err)
	}
	return result, nil
}

// HasOpenStalenessFlag reports whether an artifact has an open staleness flag.
func HasOpenStalenessFlag(ctx context.Context, pool *pgxpool.Pool, artifactID uuid.UUID, signal string) (bool, error) {
	q := db.New(pool)
	exists, err := q.HasOpenStalenessFlag(ctx, db.HasOpenStalenessFlagParams{
		ArtifactID: artifactID,
		Signal:     signal,
	})
	if err != nil {
		return false, err
	}
	return exists, nil
}

// UpdateStalenessFlag updates a staleness flag.
func UpdateStalenessFlag(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, status string, reviewedBy *uuid.UUID, note *string) error {
	q := db.New(pool)
	return q.UpdateStalenessFlag(ctx, db.UpdateStalenessFlagParams{
		ID:         id,
		Status:     status,
		ReviewedBy: reviewedBy,
		ReviewNote: note,
	})
}

// ListStalenessFlags returns staleness flags optionally filtered by status.
func ListStalenessFlags(ctx context.Context, pool *pgxpool.Pool, status string, limit, offset int) ([]*db.StalenessFlag, error) {
	q := db.New(pool)
	fs, err := q.ListStalenessFlags(ctx, db.ListStalenessFlagsParams{
		Column1: &status,
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("db: list staleness flags: %w", err)
	}
	return fs, nil
}
