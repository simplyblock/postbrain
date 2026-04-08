package jobs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/graph"
)

const defaultAGEBackfillBatchSize = 500

// AGEBackfillJob mirrors relational entities/relations into the AGE overlay.
// It is safe to run repeatedly because AGE writes are MERGE-based.
type AGEBackfillJob struct {
	pool      *pgxpool.Pool
	batchSize int
}

// NewAGEBackfillJob creates a new AGEBackfillJob.
// batchSize <= 0 defaults to 500.
func NewAGEBackfillJob(pool *pgxpool.Pool, batchSize int) *AGEBackfillJob {
	if batchSize <= 0 {
		batchSize = defaultAGEBackfillBatchSize
	}
	return &AGEBackfillJob{
		pool:      pool,
		batchSize: batchSize,
	}
}

// Run performs a full relational-to-AGE sync for entities and relations.
func (j *AGEBackfillJob) Run(ctx context.Context) error {
	if j.pool == nil {
		return fmt.Errorf("age backfill: nil pool")
	}
	if !graph.DetectAGE(ctx, j.pool) {
		slog.Info("age backfill: AGE unavailable; skipping")
		return nil
	}
	if err := db.EnsureAGEOverlay(ctx, j.pool); err != nil {
		return fmt.Errorf("age backfill: ensure overlay: %w", err)
	}

	entitiesSynced, err := j.backfillEntities(ctx)
	if err != nil {
		return err
	}
	relationsSynced, err := j.backfillRelations(ctx)
	if err != nil {
		return err
	}

	slog.Info("age backfill: complete",
		"entities_synced", entitiesSynced,
		"relations_synced", relationsSynced,
	)
	return nil
}

func (j *AGEBackfillJob) backfillEntities(ctx context.Context) (int, error) {
	offset := 0
	total := 0
	for {
		rows, err := j.pool.Query(ctx, `
			SELECT id, scope_id, entity_type, name, canonical
			FROM entities
			ORDER BY created_at, id
			LIMIT $1 OFFSET $2
		`, j.batchSize, offset)
		if err != nil {
			return total, fmt.Errorf("age backfill entities: query batch offset %d: %w", offset, err)
		}

		count := 0
		for rows.Next() {
			var e db.Entity
			if err := rows.Scan(&e.ID, &e.ScopeID, &e.EntityType, &e.Name, &e.Canonical); err != nil {
				rows.Close()
				return total, fmt.Errorf("age backfill entities: scan: %w", err)
			}
			if err := graph.SyncEntityToAGE(ctx, j.pool, &e); err != nil {
				rows.Close()
				return total, fmt.Errorf("age backfill entities: sync %s: %w", e.ID, err)
			}
			count++
			total++
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return total, fmt.Errorf("age backfill entities: rows: %w", err)
		}
		if count < j.batchSize {
			break
		}
		offset += j.batchSize
	}
	return total, nil
}

func (j *AGEBackfillJob) backfillRelations(ctx context.Context) (int, error) {
	offset := 0
	total := 0
	for {
		rows, err := j.pool.Query(ctx, `
			SELECT scope_id, subject_id, predicate, object_id, confidence
			FROM relations
			ORDER BY created_at, id
			LIMIT $1 OFFSET $2
		`, j.batchSize, offset)
		if err != nil {
			return total, fmt.Errorf("age backfill relations: query batch offset %d: %w", offset, err)
		}

		count := 0
		for rows.Next() {
			var (
				scopeID    uuid.UUID
				subjectID  uuid.UUID
				predicate  string
				objectID   uuid.UUID
				confidence float64
			)
			if err := rows.Scan(&scopeID, &subjectID, &predicate, &objectID, &confidence); err != nil {
				rows.Close()
				return total, fmt.Errorf("age backfill relations: scan: %w", err)
			}
			rel := &db.Relation{
				ScopeID:    scopeID,
				SubjectID:  subjectID,
				Predicate:  predicate,
				ObjectID:   objectID,
				Confidence: confidence,
			}
			if err := graph.SyncRelationToAGE(ctx, j.pool, rel); err != nil {
				rows.Close()
				return total, fmt.Errorf("age backfill relations: sync %s->%s %s: %w", subjectID, objectID, predicate, err)
			}
			count++
			total++
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return total, fmt.Errorf("age backfill relations: rows: %w", err)
		}
		if count < j.batchSize {
			break
		}
		offset += j.batchSize
	}
	return total, nil
}
