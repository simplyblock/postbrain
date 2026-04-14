package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/graph"
)

const defaultAGEBackfillBatchSize = 500
const ageBackfillAdvisoryLockKey int64 = 830472911245001337
const ageBackfillUnlockTimeout = 5 * time.Second

var ageBackfillTryAdvisoryLockSQL = fmt.Sprintf("SELECT pg_try_advisory_lock(%d)", ageBackfillAdvisoryLockKey)
var ageBackfillAdvisoryUnlockSQL = fmt.Sprintf("SELECT pg_advisory_unlock(%d)", ageBackfillAdvisoryLockKey)

// AGEBackfillJob mirrors relational entities/relations into the AGE overlay.
// It is intended for periodic reconciliation and uses MERGE-based AGE upserts.
type AGEBackfillJob struct {
	pool        *pgxpool.Pool
	batchSize   int
	syncEntity  func(ctx context.Context, pool *pgxpool.Pool, e *db.Entity) error
	syncRelation func(ctx context.Context, pool *pgxpool.Pool, rel *db.Relation) error
}

type ageBackfillLockConn interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// NewAGEBackfillJob creates a new AGEBackfillJob.
// batchSize <= 0 defaults to 500.
func NewAGEBackfillJob(pool *pgxpool.Pool, batchSize int) *AGEBackfillJob {
	if batchSize <= 0 {
		batchSize = defaultAGEBackfillBatchSize
	}
	return &AGEBackfillJob{
		pool:         pool,
		batchSize:    batchSize,
		syncEntity:   graph.SyncEntityToAGE,
		syncRelation: graph.SyncRelationToAGE,
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

	lockConn, err := j.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("age backfill: acquire advisory lock conn: %w", err)
	}
	defer lockConn.Release()

	locked, err := tryAcquireAGEBackfillLock(ctx, lockConn)
	if err != nil {
		return fmt.Errorf("age backfill: acquire advisory lock: %w", err)
	}
	if !locked {
		slog.Info("age backfill: previous run still active; skipping")
		return nil
	}
	defer func() {
		if unlockErr := releaseAGEBackfillLockWithTimeout(lockConn); unlockErr != nil {
			slog.Warn("age backfill: advisory unlock failed", "error", unlockErr)
		}
	}()

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

type ageEntityRow struct {
	ID         uuid.UUID
	ScopeID    uuid.UUID
	EntityType string
	Name       string
	Canonical  string
	CreatedAt  time.Time
}

func (j *AGEBackfillJob) backfillEntities(ctx context.Context) (int, error) {
	total := 0
	var (
		lastCreatedAt time.Time
		lastID        uuid.UUID
		hasCursor     bool
	)
	q := db.New(j.pool)
	for {
		var batch []ageEntityRow
		if hasCursor {
			rows, err := q.GetEntityBatchCursor(ctx, db.GetEntityBatchCursorParams{
				Column1: lastCreatedAt,
				Column2: lastID,
				Limit:   int32(j.batchSize),
			})
			if err != nil {
				return total, fmt.Errorf("age backfill entities: query batch: %w", err)
			}
			for _, r := range rows {
				batch = append(batch, ageEntityRow{r.ID, r.ScopeID, r.EntityType, r.Name, r.Canonical, r.CreatedAt})
			}
		} else {
			rows, err := q.GetEntityBatchFirstPage(ctx, int32(j.batchSize))
			if err != nil {
				return total, fmt.Errorf("age backfill entities: query batch: %w", err)
			}
			for _, r := range rows {
				batch = append(batch, ageEntityRow{r.ID, r.ScopeID, r.EntityType, r.Name, r.Canonical, r.CreatedAt})
			}
		}

		for _, r := range batch {
			e := &db.Entity{
				ID:         r.ID,
				ScopeID:    r.ScopeID,
				EntityType: r.EntityType,
				Name:       r.Name,
				Canonical:  r.Canonical,
			}
			if err := j.syncEntity(ctx, j.pool, e); err != nil {
				if errors.Is(err, graph.ErrAGEUnavailable) {
					slog.Info("age backfill entities: AGE became unavailable mid-run; aborting")
					return total, nil
				}
				return total, fmt.Errorf("age backfill entities: sync %s: %w", e.ID, err)
			}
			lastCreatedAt = r.CreatedAt
			lastID = r.ID
			hasCursor = true
			total++
		}
		if len(batch) < j.batchSize {
			break
		}
	}
	return total, nil
}

type ageRelationRow struct {
	ID         uuid.UUID
	ScopeID    uuid.UUID
	SubjectID  uuid.UUID
	Predicate  string
	ObjectID   uuid.UUID
	Confidence float64
	CreatedAt  time.Time
}

func (j *AGEBackfillJob) backfillRelations(ctx context.Context) (int, error) {
	total := 0
	var (
		lastCreatedAt time.Time
		lastID        uuid.UUID
		hasCursor     bool
	)
	q := db.New(j.pool)
	for {
		var batch []ageRelationRow
		if hasCursor {
			rows, err := q.GetRelationBatchCursor(ctx, db.GetRelationBatchCursorParams{
				Column1: lastCreatedAt,
				Column2: lastID,
				Limit:   int32(j.batchSize),
			})
			if err != nil {
				return total, fmt.Errorf("age backfill relations: query batch: %w", err)
			}
			for _, r := range rows {
				batch = append(batch, ageRelationRow{r.ID, r.ScopeID, r.SubjectID, r.Predicate, r.ObjectID, r.Confidence, r.CreatedAt})
			}
		} else {
			rows, err := q.GetRelationBatchFirstPage(ctx, int32(j.batchSize))
			if err != nil {
				return total, fmt.Errorf("age backfill relations: query batch: %w", err)
			}
			for _, r := range rows {
				batch = append(batch, ageRelationRow{r.ID, r.ScopeID, r.SubjectID, r.Predicate, r.ObjectID, r.Confidence, r.CreatedAt})
			}
		}

		for _, r := range batch {
			rel := &db.Relation{
				ScopeID:    r.ScopeID,
				SubjectID:  r.SubjectID,
				Predicate:  r.Predicate,
				ObjectID:   r.ObjectID,
				Confidence: r.Confidence,
			}
			if err := j.syncRelation(ctx, j.pool, rel); err != nil {
				if errors.Is(err, graph.ErrAGEUnavailable) {
					slog.Info("age backfill relations: AGE became unavailable mid-run; aborting")
					return total, nil
				}
				if shouldSkipAGEBackfillRelationSyncError(err) {
					slog.Warn("age backfill relations: skipping relation after AGE internal update failure",
						"subject_id", r.SubjectID,
						"object_id", r.ObjectID,
						"predicate", r.Predicate,
						"error", err,
					)
					lastCreatedAt = r.CreatedAt
					lastID = r.ID
					hasCursor = true
					continue
				}
				return total, fmt.Errorf("age backfill relations: sync %s->%s %s: %w", r.SubjectID, r.ObjectID, r.Predicate, err)
			}
			lastCreatedAt = r.CreatedAt
			lastID = r.ID
			hasCursor = true
			total++
		}
		if len(batch) < j.batchSize {
			break
		}
	}
	return total, nil
}

func tryAcquireAGEBackfillLock(ctx context.Context, conn ageBackfillLockConn) (bool, error) {
	var locked bool
	if err := conn.QueryRow(ctx, ageBackfillTryAdvisoryLockSQL).Scan(&locked); err != nil {
		return false, err
	}
	return locked, nil
}

func releaseAGEBackfillLock(ctx context.Context, conn ageBackfillLockConn) error {
	var unlocked bool
	if err := conn.QueryRow(ctx, ageBackfillAdvisoryUnlockSQL).Scan(&unlocked); err != nil {
		return err
	}
	if !unlocked {
		return fmt.Errorf("advisory lock %d was not held", ageBackfillAdvisoryLockKey)
	}
	return nil
}

func releaseAGEBackfillLockWithTimeout(conn ageBackfillLockConn) error {
	unlockCtx, cancel := context.WithTimeout(context.Background(), ageBackfillUnlockTimeout)
	defer cancel()
	return releaseAGEBackfillLock(unlockCtx, conn)
}

func shouldSkipAGEBackfillRelationSyncError(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "XX000" && strings.Contains(pgErr.Message, "Entity failed to be updated")
}
