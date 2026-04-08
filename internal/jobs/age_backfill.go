package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/graph"
)

const defaultAGEBackfillBatchSize = 500

const ageBackfillEntityBatchFirstPageSQL = `
	SELECT id, scope_id, entity_type, name, canonical, created_at
	FROM entities
	ORDER BY created_at, id
	LIMIT $1
`

const ageBackfillEntityBatchCursorSQL = `
	SELECT id, scope_id, entity_type, name, canonical, created_at
	FROM entities
	WHERE (created_at, id) > ($1, $2)
	ORDER BY created_at, id
	LIMIT $3
`

const ageBackfillRelationBatchFirstPageSQL = `
	SELECT id, scope_id, subject_id, predicate, object_id, confidence, created_at
	FROM relations
	ORDER BY created_at, id
	LIMIT $1
`

const ageBackfillRelationBatchCursorSQL = `
	SELECT id, scope_id, subject_id, predicate, object_id, confidence, created_at
	FROM relations
	WHERE (created_at, id) > ($1, $2)
	ORDER BY created_at, id
	LIMIT $3
`

// AGEBackfillJob mirrors relational entities/relations into the AGE overlay.
// It is intended for periodic reconciliation and currently relies on
// match-then-create AGE sync helpers (not Cypher MERGE upserts).
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
	if err := db.EnsureAGEOverlay(ctx, j.pool); err != nil {
		return fmt.Errorf("age backfill: ensure overlay: %w", err)
	}
	if !graph.DetectAGE(ctx, j.pool) {
		slog.Info("age backfill: AGE unavailable; skipping")
		return nil
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
	total := 0
	var (
		lastCreatedAt time.Time
		lastID        uuid.UUID
		hasCursor     bool
	)
	for {
		query := entityBatchQuery(hasCursor)
		args := []any{j.batchSize}
		if hasCursor {
			args = []any{lastCreatedAt, lastID, j.batchSize}
		}
		rows, err := j.pool.Query(ctx, query, args...)
		if err != nil {
			return total, fmt.Errorf("age backfill entities: query batch: %w", err)
		}

		count := 0
		for rows.Next() {
			var e db.Entity
			var createdAt time.Time
			if err := rows.Scan(&e.ID, &e.ScopeID, &e.EntityType, &e.Name, &e.Canonical, &createdAt); err != nil {
				rows.Close()
				return total, fmt.Errorf("age backfill entities: scan: %w", err)
			}
			if err := graph.SyncEntityToAGE(ctx, j.pool, &e); err != nil {
				rows.Close()
				return total, fmt.Errorf("age backfill entities: sync %s: %w", e.ID, err)
			}
			lastCreatedAt = createdAt
			lastID = e.ID
			hasCursor = true
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
	}
	return total, nil
}

func (j *AGEBackfillJob) backfillRelations(ctx context.Context) (int, error) {
	total := 0
	var (
		lastCreatedAt time.Time
		lastID        uuid.UUID
		hasCursor     bool
	)
	for {
		query := relationBatchQuery(hasCursor)
		args := []any{j.batchSize}
		if hasCursor {
			args = []any{lastCreatedAt, lastID, j.batchSize}
		}
		rows, err := j.pool.Query(ctx, query, args...)
		if err != nil {
			return total, fmt.Errorf("age backfill relations: query batch: %w", err)
		}

		count := 0
		for rows.Next() {
			var (
				id         uuid.UUID
				scopeID    uuid.UUID
				subjectID  uuid.UUID
				predicate  string
				objectID   uuid.UUID
				confidence float64
				createdAt  time.Time
			)
			if err := rows.Scan(&id, &scopeID, &subjectID, &predicate, &objectID, &confidence, &createdAt); err != nil {
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
			lastCreatedAt = createdAt
			lastID = id
			hasCursor = true
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
	}
	return total, nil
}

func entityBatchQuery(hasCursor bool) string {
	if hasCursor {
		return ageBackfillEntityBatchCursorSQL
	}
	return ageBackfillEntityBatchFirstPageSQL
}

func relationBatchQuery(hasCursor bool) string {
	if hasCursor {
		return ageBackfillRelationBatchCursorSQL
	}
	return ageBackfillRelationBatchFirstPageSQL
}
