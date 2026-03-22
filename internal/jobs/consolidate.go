package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/embedding"
	"github.com/simplyblock/postbrain/internal/memory"
)

// ConsolidateJob runs LLM-assisted memory consolidation for all scopes.
type ConsolidateJob struct {
	pool      *pgxpool.Pool
	svc       *embedding.EmbeddingService
	summarize func(ctx context.Context, contents []string) (string, error)
}

// NewConsolidateJob creates a new ConsolidateJob. If summarize is nil, a default
// summarizer that concatenates contents with a separator is used.
func NewConsolidateJob(pool *pgxpool.Pool, svc *embedding.EmbeddingService, summarize func(ctx context.Context, contents []string) (string, error)) *ConsolidateJob {
	if summarize == nil {
		summarize = defaultSummarizer
	}
	return &ConsolidateJob{
		pool:      pool,
		svc:       svc,
		summarize: summarize,
	}
}

// Run finds and merges near-duplicate memory clusters across all active scopes.
func (j *ConsolidateJob) Run(ctx context.Context) error {
	// Fetch all distinct scope IDs that have consolidation candidates.
	rows, err := j.pool.Query(ctx,
		`SELECT DISTINCT scope_id FROM memories
		 WHERE is_active=true AND importance < 0.7 AND access_count < 3`,
	)
	if err != nil {
		return fmt.Errorf("consolidate: fetch scopes: %w", err)
	}
	defer rows.Close()

	var scopeIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("consolidate: scan scope_id: %w", err)
		}
		scopeIDs = append(scopeIDs, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("consolidate: rows error: %w", err)
	}

	totalMerged := 0
	for _, scopeID := range scopeIDs {
		consolidator := memory.NewConsolidator(j.pool, j.svc)
		clusters, err := consolidator.FindClusters(ctx, scopeID)
		if err != nil {
			slog.Error("consolidate: find clusters failed", "scope_id", scopeID, "error", err)
			continue
		}
		for _, cluster := range clusters {
			if _, err := consolidator.MergeCluster(ctx, cluster, j.summarize); err != nil {
				slog.Error("consolidate: merge cluster failed", "scope_id", scopeID, "error", err)
				continue
			}
			totalMerged++
		}
	}

	slog.Info("consolidate: complete", "scopes_processed", len(scopeIDs), "clusters_merged", totalMerged)
	return nil
}

// defaultSummarizer is the fallback summarizer used when no LLM summarizer is
// provided. It concatenates contents with a separator — no external API call required.
func defaultSummarizer(_ context.Context, contents []string) (string, error) {
	return strings.Join(contents, "\n---\n"), nil
}
