package jobs

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
)

const defaultReembedBatchSize = 64
const maxEmbeddingRetries = 3

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
		`SELECT id FROM ai_models WHERE is_active=true AND model_type='embedding' AND content_type=$1`,
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

	total := 0
	for {
		rows, err := j.pool.Query(ctx, `
			SELECT ei.object_type, ei.object_id, ei.retry_count,
			       CASE
			           WHEN ei.object_type='skill' THEN btrim(concat_ws(' ', NULLIF(s.description, ''), NULLIF(s.body, '')))
			           ELSE COALESCE(m.content, ka.content, s.body)
			       END AS content
			FROM embedding_index ei
			LEFT JOIN memories m ON ei.object_type='memory' AND m.id=ei.object_id AND m.is_active=true
			LEFT JOIN knowledge_artifacts ka ON ei.object_type='knowledge_artifact' AND ka.id=ei.object_id
			LEFT JOIN skills s ON ei.object_type='skill' AND s.id=ei.object_id
			WHERE ei.model_id = $1
			  AND ei.status = 'pending'
			  AND ei.object_type IN ('memory','knowledge_artifact','skill')
			ORDER BY ei.updated_at, ei.object_id
			LIMIT $2
		`, modelID, j.batchSize)
		if err != nil {
			return fmt.Errorf("reembed text: fetch pending batch: %w", err)
		}

		type row struct {
			objectType string
			id         uuid.UUID
			retryCount int
			content    sql.NullString
		}
		var batch []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.objectType, &r.id, &r.retryCount, &r.content); err != nil {
				rows.Close()
				return fmt.Errorf("reembed text: scan row: %w", err)
			}
			if strings.TrimSpace(r.content.String) == "" {
				_ = j.markEmbeddingFailedAttempt(ctx, r.objectType, r.id, *modelID, r.retryCount, fmt.Errorf("empty content for %s %v", r.objectType, r.id))
				continue
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("reembed text: rows error: %w", err)
		}

		if len(batch) == 0 {
			break
		}

		for _, r := range batch {
			vec, err := j.svc.EmbedText(ctx, r.content.String)
			if err != nil {
				_ = j.markEmbeddingFailedAttempt(ctx, r.objectType, r.id, *modelID, r.retryCount, err)
				slog.Error("reembed text: embed failed", "object_type", r.objectType, "object_id", r.id, "error", err)
				continue
			}
			if err := j.updateTextEmbeddingByObjectType(ctx, r.objectType, r.id, vec, modelID); err != nil {
				_ = j.markEmbeddingFailedAttempt(ctx, r.objectType, r.id, *modelID, r.retryCount, err)
				slog.Error("reembed text: update failed", "object_type", r.objectType, "object_id", r.id, "error", err)
				continue
			}
			scopeID := j.resolveScopeID(ctx, r.objectType, r.id)
			if scopeID == uuid.Nil {
				_ = j.markEmbeddingFailedAttempt(ctx, r.objectType, r.id, *modelID, r.retryCount, fmt.Errorf("missing scope_id for %s %s", r.objectType, r.id))
				continue
			}
			if err := db.NewEmbeddingRepository(j.pool).UpsertEmbedding(ctx, db.UpsertEmbeddingInput{
				ObjectType: r.objectType,
				ObjectID:   r.id,
				ScopeID:    scopeID,
				ModelID:    *modelID,
				Embedding:  vec,
			}); err != nil {
				_ = j.markEmbeddingFailedAttempt(ctx, r.objectType, r.id, *modelID, r.retryCount, err)
				slog.Error("reembed text: repository upsert failed", "object_type", r.objectType, "object_id", r.id, "error", err)
				continue
			}
			if err := j.markEmbeddingReady(ctx, r.objectType, r.id, *modelID); err != nil {
				slog.Error("reembed text: mark ready failed", "object_type", r.objectType, "object_id", r.id, "error", err)
			}
		}

		total += len(batch)
		slog.Info("reembed text: batch processed",
			"count", len(batch), "total_so_far", total)

		if len(batch) < j.batchSize {
			break
		}
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

	total := 0
	for {
		rows, err := j.pool.Query(ctx, `
			SELECT ei.object_id, ei.retry_count, m.content, m.scope_id
			FROM embedding_index ei
			JOIN memories m ON ei.object_type='memory' AND m.id=ei.object_id
			WHERE ei.model_id = $1
			  AND ei.status = 'pending'
			  AND m.is_active = true
			  AND m.content_kind = 'code'
			ORDER BY ei.updated_at, ei.object_id
			LIMIT $2
		`, modelID, j.batchSize)
		if err != nil {
			return fmt.Errorf("reembed code: fetch pending batch: %w", err)
		}

		type row struct {
			id         uuid.UUID
			retryCount int
			content    string
			scopeID    uuid.UUID
		}
		var batch []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.retryCount, &r.content, &r.scopeID); err != nil {
				rows.Close()
				return fmt.Errorf("reembed code: scan row: %w", err)
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("reembed code: rows error: %w", err)
		}

		if len(batch) == 0 {
			break
		}

		for _, r := range batch {
			vec, err := j.svc.EmbedCode(ctx, r.content)
			if err != nil {
				_ = j.markEmbeddingFailedAttempt(ctx, "memory", r.id, *modelID, r.retryCount, err)
				slog.Error("reembed code: embed failed", "memory_id", r.id, "error", err)
				continue
			}
			if err := j.updateMemoryCodeEmbedding(ctx, r.id, vec, modelID); err != nil {
				_ = j.markEmbeddingFailedAttempt(ctx, "memory", r.id, *modelID, r.retryCount, err)
				slog.Error("reembed code: update failed", "memory_id", r.id, "error", err)
				continue
			}
			if err := db.NewEmbeddingRepository(j.pool).UpsertEmbedding(ctx, db.UpsertEmbeddingInput{
				ObjectType: "memory",
				ObjectID:   r.id,
				ScopeID:    r.scopeID,
				ModelID:    *modelID,
				Embedding:  vec,
			}); err != nil {
				_ = j.markEmbeddingFailedAttempt(ctx, "memory", r.id, *modelID, r.retryCount, err)
				slog.Error("reembed code: repository upsert failed", "memory_id", r.id, "error", err)
				continue
			}
			if err := j.markEmbeddingReady(ctx, "memory", r.id, *modelID); err != nil {
				slog.Error("reembed code: mark ready failed", "memory_id", r.id, "error", err)
			}
		}

		total += len(batch)
		slog.Info("reembed code: batch processed",
			"count", len(batch), "total_so_far", total)

		if len(batch) < j.batchSize {
			break
		}
	}

	slog.Info("reembed code: complete", "total_reembedded", total, "model_id", modelID)
	return nil
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

func (j *ReembedJob) updateTextEmbeddingByObjectType(ctx context.Context, objectType string, id uuid.UUID, vec []float32, modelID *uuid.UUID) error {
	vecStr := db.ExportFloat32SliceToVector(vec)
	switch objectType {
	case "memory":
		_, err := j.pool.Exec(ctx, `UPDATE memories SET embedding=$2::vector, embedding_model_id=$3, updated_at=now() WHERE id=$1`, id, vecStr, modelID)
		return err
	case "knowledge_artifact":
		_, err := j.pool.Exec(ctx, `UPDATE knowledge_artifacts SET embedding=$2::vector, embedding_model_id=$3, updated_at=now() WHERE id=$1`, id, vecStr, modelID)
		return err
	case "skill":
		_, err := j.pool.Exec(ctx, `UPDATE skills SET embedding=$2::vector, embedding_model_id=$3, updated_at=now() WHERE id=$1`, id, vecStr, modelID)
		return err
	default:
		return fmt.Errorf("unsupported object_type %q", objectType)
	}
}

func (j *ReembedJob) resolveScopeID(ctx context.Context, objectType string, id uuid.UUID) uuid.UUID {
	var scopeID uuid.UUID
	switch objectType {
	case "memory", "skill":
		_ = j.pool.QueryRow(ctx, `SELECT scope_id FROM `+map[string]string{"memory": "memories", "skill": "skills"}[objectType]+` WHERE id=$1`, id).Scan(&scopeID)
	case "knowledge_artifact":
		_ = j.pool.QueryRow(ctx, `SELECT owner_scope_id FROM knowledge_artifacts WHERE id=$1`, id).Scan(&scopeID)
	}
	return scopeID
}

func (j *ReembedJob) markEmbeddingReady(ctx context.Context, objectType string, objectID, modelID uuid.UUID) error {
	_, err := j.pool.Exec(ctx, `
		UPDATE embedding_index
		SET status='ready', retry_count=0, last_error=NULL, updated_at=now()
		WHERE object_type=$1 AND object_id=$2 AND model_id=$3
	`, objectType, objectID, modelID)
	return err
}

func (j *ReembedJob) markEmbeddingFailedAttempt(ctx context.Context, objectType string, objectID, modelID uuid.UUID, currentRetry int, cause error) error {
	nextRetry := currentRetry + 1
	status := "pending"
	if nextRetry >= maxEmbeddingRetries {
		status = "failed"
	}
	_, err := j.pool.Exec(ctx, `
		UPDATE embedding_index
		SET status=$4, retry_count=$5, last_error=$6, updated_at=now()
		WHERE object_type=$1 AND object_id=$2 AND model_id=$3
	`, objectType, objectID, modelID, status, nextRetry, cause.Error())
	return err
}
