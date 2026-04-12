package jobs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/embedding"
	"github.com/simplyblock/postbrain/internal/knowledge"
)

// backfillRow holds the minimal fields needed to generate a summary.
type backfillRow struct {
	ID      uuid.UUID
	Content string
}

// backfillSummaryStore abstracts DB access for BackfillSummariesJob.
type backfillSummaryStore interface {
	fetchUnsummarised(ctx context.Context, batchSize, offset int) ([]backfillRow, error)
	setSummary(ctx context.Context, id uuid.UUID, summary string) error
}

// poolBackfillStore implements backfillSummaryStore against a real pgxpool.
type poolBackfillStore struct {
	pool *pgxpool.Pool
}

func (p *poolBackfillStore) fetchUnsummarised(ctx context.Context, batchSize, offset int) ([]backfillRow, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, content FROM knowledge_artifacts
		 WHERE summary IS NULL
		 ORDER BY created_at
		 LIMIT $1 OFFSET $2`,
		batchSize, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var batch []backfillRow
	for rows.Next() {
		var r backfillRow
		if err := rows.Scan(&r.ID, &r.Content); err != nil {
			return nil, err
		}
		batch = append(batch, r)
	}
	return batch, rows.Err()
}

func (p *poolBackfillStore) setSummary(ctx context.Context, id uuid.UUID, summary string) error {
	_, err := p.pool.Exec(ctx,
		`UPDATE knowledge_artifacts SET summary=$2, updated_at=now() WHERE id=$1`,
		id, summary,
	)
	return err
}

const defaultBackfillBatchSize = 50

// BackfillSummariesJob scans knowledge_artifacts with a NULL summary and fills
// them using AI summarization when available, falling back to extractive.
type BackfillSummariesJob struct {
	store      backfillSummaryStore
	summarizer embedding.Summarizer // may be nil → extractive fallback
	batchSize  int
}

// NewBackfillSummariesJob creates a BackfillSummariesJob backed by pool.
// svc may be nil; if non-nil and a summary model is configured, AI summarization
// is used. If batchSize is 0 it defaults to 50.
func NewBackfillSummariesJob(pool *pgxpool.Pool, svc *embedding.EmbeddingService, batchSize int) *BackfillSummariesJob {
	if batchSize <= 0 {
		batchSize = defaultBackfillBatchSize
	}
	j := &BackfillSummariesJob{
		store:     &poolBackfillStore{pool: pool},
		batchSize: batchSize,
	}
	if svc != nil {
		j.summarizer = svc
	}
	return j
}

// Run processes all unsummarised artifacts in batches.
func (j *BackfillSummariesJob) Run(ctx context.Context) error {
	updated := 0
	_, err := RunPaginatedBatch(ctx, j.batchSize, j.store.fetchUnsummarised,
		func(ctx context.Context, r backfillRow) {
			sum := j.generateSummary(ctx, r.Content)
			if setErr := j.store.setSummary(ctx, r.ID, sum); setErr != nil {
				slog.Error("backfill summaries: update failed", "artifact_id", r.ID, "error", setErr)
				return
			}
			updated++
		},
	)
	if err != nil {
		return fmt.Errorf("backfill summaries: %w", err)
	}
	slog.Info("backfill summaries: complete", "total_updated", updated)
	return nil
}

// generateSummary tries AI summarization first, falls back to extractive.
func (j *BackfillSummariesJob) generateSummary(ctx context.Context, content string) string {
	if j.summarizer != nil {
		if sum, err := j.summarizer.Summarize(ctx, content); err == nil && sum != "" {
			return sum
		}
	}
	return knowledge.Summarize(content, 150)
}
