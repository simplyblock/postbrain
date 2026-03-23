package jobs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
)

const defaultReembedBatchSize = 64

// ReembedJob re-embeds memories and knowledge artifacts whose embedding_model_id
// does not match the current active model.
type ReembedJob struct {
	pool      *pgxpool.Pool
	svc       *embedding.EmbeddingService
	batchSize int
}

// NewReembedJob creates a new ReembedJob. If batchSize is 0, it defaults to 64.
func NewReembedJob(pool *pgxpool.Pool, svc *embedding.EmbeddingService, batchSize int) *ReembedJob {
	if batchSize <= 0 {
		batchSize = defaultReembedBatchSize
	}
	return &ReembedJob{
		pool:      pool,
		svc:       svc,
		batchSize: batchSize,
	}
}

// activeModelID fetches the UUID of the active embedding model for the given content type.
// Returns nil (and no error) if no active model is registered.
func (j *ReembedJob) activeModelID(ctx context.Context, contentType string) (*uuid.UUID, error) {
	var id uuid.UUID
	err := j.pool.QueryRow(ctx,
		`SELECT id FROM embedding_models WHERE is_active=true AND content_type=$1`,
		contentType,
	).Scan(&id)
	if err != nil {
		// No active model is not an error — just nothing to do.
		return nil, nil //nolint:nilerr
	}
	return &id, nil
}

// RunText re-embeds all memories/artifacts using the text model where embedding_model_id differs.
func (j *ReembedJob) RunText(ctx context.Context) error {
	modelID, err := j.activeModelID(ctx, "text")
	if err != nil {
		return fmt.Errorf("reembed text: fetch active model: %w", err)
	}
	if modelID == nil {
		slog.Warn("reembed text: no active text model; skipping")
		return nil
	}

	offset := 0
	total := 0
	for {
		rows, err := j.pool.Query(ctx,
			`SELECT id, content FROM memories
			 WHERE is_active=true
			   AND (embedding_model_id IS NULL OR embedding_model_id != $1)
			 LIMIT $2 OFFSET $3`,
			modelID, j.batchSize, offset,
		)
		if err != nil {
			return fmt.Errorf("reembed text: fetch batch at offset %d: %w", offset, err)
		}

		type row struct {
			id      uuid.UUID
			content string
		}
		var batch []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.content); err != nil {
				rows.Close()
				return fmt.Errorf("reembed text: scan row: %w", err)
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("reembed text: rows error at offset %d: %w", offset, err)
		}

		if len(batch) == 0 {
			break
		}

		for _, r := range batch {
			vec, err := j.svc.EmbedText(ctx, r.content)
			if err != nil {
				slog.Error("reembed text: embed failed", "memory_id", r.id, "error", err)
				continue
			}
			if err := j.updateMemoryTextEmbedding(ctx, r.id, vec, modelID); err != nil {
				slog.Error("reembed text: update failed", "memory_id", r.id, "error", err)
			}
		}

		total += len(batch)
		slog.Info("reembed text: batch processed",
			"offset", offset, "count", len(batch), "total_so_far", total)

		if len(batch) < j.batchSize {
			break
		}
		offset += j.batchSize
	}

	slog.Info("reembed text: complete", "total_reembedded", total, "model_id", modelID)
	return nil
}

// RunCode re-embeds all code-kind memories using the code model where embedding_code_model_id differs.
func (j *ReembedJob) RunCode(ctx context.Context) error {
	modelID, err := j.activeModelID(ctx, "code")
	if err != nil {
		return fmt.Errorf("reembed code: fetch active model: %w", err)
	}
	if modelID == nil {
		slog.Warn("reembed code: no active code model; skipping")
		return nil
	}

	offset := 0
	total := 0
	for {
		rows, err := j.pool.Query(ctx,
			`SELECT id, content FROM memories
			 WHERE is_active=true AND content_kind='code'
			   AND (embedding_code_model_id IS NULL OR embedding_code_model_id != $1)
			 LIMIT $2 OFFSET $3`,
			modelID, j.batchSize, offset,
		)
		if err != nil {
			return fmt.Errorf("reembed code: fetch batch at offset %d: %w", offset, err)
		}

		type row struct {
			id      uuid.UUID
			content string
		}
		var batch []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.content); err != nil {
				rows.Close()
				return fmt.Errorf("reembed code: scan row: %w", err)
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("reembed code: rows error at offset %d: %w", offset, err)
		}

		if len(batch) == 0 {
			break
		}

		for _, r := range batch {
			vec, err := j.svc.EmbedCode(ctx, r.content)
			if err != nil {
				slog.Error("reembed code: embed failed", "memory_id", r.id, "error", err)
				continue
			}
			if err := j.updateMemoryCodeEmbedding(ctx, r.id, vec, modelID); err != nil {
				slog.Error("reembed code: update failed", "memory_id", r.id, "error", err)
			}
		}

		total += len(batch)
		slog.Info("reembed code: batch processed",
			"offset", offset, "count", len(batch), "total_so_far", total)

		if len(batch) < j.batchSize {
			break
		}
		offset += j.batchSize
	}

	slog.Info("reembed code: complete", "total_reembedded", total, "model_id", modelID)
	return nil
}

// updateMemoryTextEmbedding updates the text embedding and model ID for a memory.
func (j *ReembedJob) updateMemoryTextEmbedding(ctx context.Context, id uuid.UUID, vec []float32, modelID *uuid.UUID) error {
	vecStr := db.ExportFloat32SliceToVector(vec)
	_, err := j.pool.Exec(ctx,
		`UPDATE memories SET embedding=$2::vector, embedding_model_id=$3, updated_at=now()
		 WHERE id=$1`,
		id, vecStr, modelID,
	)
	return err
}

// updateMemoryCodeEmbedding updates the code embedding and model ID for a memory.
func (j *ReembedJob) updateMemoryCodeEmbedding(ctx context.Context, id uuid.UUID, vec []float32, modelID *uuid.UUID) error {
	vecStr := db.ExportFloat32SliceToVector(vec)
	_, err := j.pool.Exec(ctx,
		`UPDATE memories SET embedding_code=$2::vector, embedding_code_model_id=$3, updated_at=now()
		 WHERE id=$1`,
		id, vecStr, modelID,
	)
	return err
}
