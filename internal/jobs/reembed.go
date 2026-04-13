package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/providers"
)

const defaultReembedBatchSize = 64
const maxEmbeddingRetries = 3

// ReembedJob re-embeds memories and knowledge artifacts whose embedding_model_id
// does not match the current active model.
type ReembedJob struct {
	pool      *pgxpool.Pool
	svc       *providers.EmbeddingService
	batchSize int
}

// NewReembedJob creates a new ReembedJob. If batchSize is 0, it defaults to 64.
func NewReembedJob(pool *pgxpool.Pool, svc *providers.EmbeddingService, batchSize int) *ReembedJob {
	if batchSize <= 0 {
		batchSize = defaultReembedBatchSize
	}
	return &ReembedJob{
		pool:      pool,
		svc:       svc,
		batchSize: batchSize,
	}
}

// RunText re-embeds all memories/artifacts using the text model where embedding_model_id differs.
func (j *ReembedJob) RunText(ctx context.Context) error {
	model, err := db.New(j.pool).GetActiveTextModel(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		slog.Warn("reembed text: no active text model; skipping")
		return nil
	}
	if err != nil {
		return fmt.Errorf("reembed text: fetch active model: %w", err)
	}
	modelID := model.ID

	total := 0
	for {
		batch, err := db.New(j.pool).GetPendingTextEmbeddingBatch(ctx, db.GetPendingTextEmbeddingBatchParams{
			ModelID: modelID,
			Limit:   int32(j.batchSize),
		})
		if err != nil {
			return fmt.Errorf("reembed text: fetch pending batch: %w", err)
		}

		// Filter empty-content rows and mark them failed before processing.
		// Content is interface{} (sqlc limitation for CASE/COALESCE); pgx always
		// returns string for text columns, so the assertion is safe at runtime.
		var active []*db.GetPendingTextEmbeddingBatchRow
		for _, r := range batch {
			content, _ := r.Content.(string)
			if strings.TrimSpace(content) == "" {
				if err := j.markEmbeddingFailedAttempt(ctx, r.ObjectType, r.ObjectID, modelID, int(r.RetryCount), fmt.Errorf("empty content for %s %v", r.ObjectType, r.ObjectID)); err != nil {
					slog.Error("reembed text: mark failed attempt error", "object_id", r.ObjectID, "error", err)
				}
				continue
			}
			active = append(active, r)
		}

		if len(active) == 0 {
			break
		}

		for _, r := range active {
			content, _ := r.Content.(string)
			vec, err := j.svc.EmbedText(ctx, content)
			if err != nil {
				if markErr := j.markEmbeddingFailedAttempt(ctx, r.ObjectType, r.ObjectID, modelID, int(r.RetryCount), err); markErr != nil {
					slog.Error("reembed text: mark failed attempt error", "object_id", r.ObjectID, "error", markErr)
				}
				slog.Error("reembed text: embed failed", "object_type", r.ObjectType, "object_id", r.ObjectID, "error", err)
				continue
			}
			if err := j.updateTextEmbeddingByObjectType(ctx, r.ObjectType, r.ObjectID, vec, modelID); err != nil {
				if markErr := j.markEmbeddingFailedAttempt(ctx, r.ObjectType, r.ObjectID, modelID, int(r.RetryCount), err); markErr != nil {
					slog.Error("reembed text: mark failed attempt error", "object_id", r.ObjectID, "error", markErr)
				}
				slog.Error("reembed text: update failed", "object_type", r.ObjectType, "object_id", r.ObjectID, "error", err)
				continue
			}
			scopeID := j.resolveScopeID(ctx, r.ObjectType, r.ObjectID)
			if scopeID == uuid.Nil {
				if markErr := j.markEmbeddingFailedAttempt(ctx, r.ObjectType, r.ObjectID, modelID, int(r.RetryCount), fmt.Errorf("missing scope_id for %s %s", r.ObjectType, r.ObjectID)); markErr != nil {
					slog.Error("reembed text: mark failed attempt error", "object_id", r.ObjectID, "error", markErr)
				}
				continue
			}
			if err := db.NewEmbeddingRepository(j.pool).UpsertEmbedding(ctx, db.UpsertEmbeddingInput{
				ObjectType: r.ObjectType,
				ObjectID:   r.ObjectID,
				ScopeID:    scopeID,
				ModelID:    modelID,
				Embedding:  vec,
			}); err != nil {
				if markErr := j.markEmbeddingFailedAttempt(ctx, r.ObjectType, r.ObjectID, modelID, int(r.RetryCount), err); markErr != nil {
					slog.Error("reembed text: mark failed attempt error", "object_id", r.ObjectID, "error", markErr)
				}
				slog.Error("reembed text: repository upsert failed", "object_type", r.ObjectType, "object_id", r.ObjectID, "error", err)
				continue
			}
			if err := j.markEmbeddingReady(ctx, r.ObjectType, r.ObjectID, modelID); err != nil {
				slog.Error("reembed text: mark ready failed", "object_type", r.ObjectType, "object_id", r.ObjectID, "error", err)
			}
		}

		total += len(active)
		slog.Info("reembed text: batch processed",
			"count", len(active), "total_so_far", total)

		if len(batch) < j.batchSize {
			break
		}
	}

	slog.Info("reembed text: complete", "total_reembedded", total, "model_id", modelID)
	return nil
}

// RunCode re-embeds all code-kind memories using the code model where embedding_code_model_id differs.
func (j *ReembedJob) RunCode(ctx context.Context) error {
	model, err := db.New(j.pool).GetActiveCodeModel(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		slog.Warn("reembed code: no active code model; skipping")
		return nil
	}
	if err != nil {
		return fmt.Errorf("reembed code: fetch active model: %w", err)
	}
	modelID := model.ID

	total := 0
	for {
		batch, err := db.New(j.pool).GetPendingCodeEmbeddingBatch(ctx, db.GetPendingCodeEmbeddingBatchParams{
			ModelID: modelID,
			Limit:   int32(j.batchSize),
		})
		if err != nil {
			return fmt.Errorf("reembed code: fetch pending batch: %w", err)
		}
		if len(batch) == 0 {
			break
		}

		for _, r := range batch {
			vec, err := j.svc.EmbedCode(ctx, r.Content)
			if err != nil {
				if markErr := j.markEmbeddingFailedAttempt(ctx, "memory", r.ObjectID, modelID, int(r.RetryCount), err); markErr != nil {
					slog.Error("reembed code: mark failed attempt error", "memory_id", r.ObjectID, "error", markErr)
				}
				slog.Error("reembed code: embed failed", "memory_id", r.ObjectID, "error", err)
				continue
			}
			v := pgvector.NewVector(vec)
			if err := db.New(j.pool).UpdateMemoryCodeEmbedding(ctx, db.UpdateMemoryCodeEmbeddingParams{
				ID:                   r.ObjectID,
				EmbeddingCode:        &v,
				EmbeddingCodeModelID: &modelID,
			}); err != nil {
				if markErr := j.markEmbeddingFailedAttempt(ctx, "memory", r.ObjectID, modelID, int(r.RetryCount), err); markErr != nil {
					slog.Error("reembed code: mark failed attempt error", "memory_id", r.ObjectID, "error", markErr)
				}
				slog.Error("reembed code: update failed", "memory_id", r.ObjectID, "error", err)
				continue
			}
			if err := db.NewEmbeddingRepository(j.pool).UpsertEmbedding(ctx, db.UpsertEmbeddingInput{
				ObjectType: "memory",
				ObjectID:   r.ObjectID,
				ScopeID:    r.ScopeID,
				ModelID:    modelID,
				Embedding:  vec,
			}); err != nil {
				if markErr := j.markEmbeddingFailedAttempt(ctx, "memory", r.ObjectID, modelID, int(r.RetryCount), err); markErr != nil {
					slog.Error("reembed code: mark failed attempt error", "memory_id", r.ObjectID, "error", markErr)
				}
				slog.Error("reembed code: repository upsert failed", "memory_id", r.ObjectID, "error", err)
				continue
			}
			if err := j.markEmbeddingReady(ctx, "memory", r.ObjectID, modelID); err != nil {
				slog.Error("reembed code: mark ready failed", "memory_id", r.ObjectID, "error", err)
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

func (j *ReembedJob) updateTextEmbeddingByObjectType(ctx context.Context, objectType string, id uuid.UUID, vec []float32, modelID uuid.UUID) error {
	v := pgvector.NewVector(vec)
	switch objectType {
	case "memory":
		return db.New(j.pool).UpdateMemoryTextEmbedding(ctx, db.UpdateMemoryTextEmbeddingParams{
			ID:               id,
			Embedding:        &v,
			EmbeddingModelID: &modelID,
		})
	case "knowledge_artifact":
		return db.New(j.pool).UpdateKnowledgeArtifactEmbedding(ctx, db.UpdateKnowledgeArtifactEmbeddingParams{
			ID:               id,
			Embedding:        &v,
			EmbeddingModelID: &modelID,
		})
	case "skill":
		return db.New(j.pool).UpdateSkillEmbedding(ctx, db.UpdateSkillEmbeddingParams{
			ID:               id,
			Embedding:        &v,
			EmbeddingModelID: &modelID,
		})
	default:
		return fmt.Errorf("unsupported object_type %q", objectType)
	}
}

func (j *ReembedJob) resolveScopeID(ctx context.Context, objectType string, id uuid.UUID) uuid.UUID {
	q := db.New(j.pool)
	switch objectType {
	case "memory":
		scopeID, err := q.GetMemoryScopeID(ctx, id)
		if err != nil {
			return uuid.Nil
		}
		return scopeID
	case "skill":
		scopeID, err := q.GetSkillScopeID(ctx, id)
		if err != nil {
			return uuid.Nil
		}
		return scopeID
	case "knowledge_artifact":
		scopeID, err := q.GetArtifactOwnerScopeID(ctx, id)
		if err != nil {
			return uuid.Nil
		}
		return scopeID
	}
	return uuid.Nil
}

func (j *ReembedJob) markEmbeddingReady(ctx context.Context, objectType string, objectID, modelID uuid.UUID) error {
	return db.New(j.pool).MarkEmbeddingIndexReady(ctx, db.MarkEmbeddingIndexReadyParams{
		ObjectType: objectType,
		ObjectID:   objectID,
		ModelID:    modelID,
	})
}

func (j *ReembedJob) markEmbeddingFailedAttempt(ctx context.Context, objectType string, objectID, modelID uuid.UUID, currentRetry int, cause error) error {
	nextRetry := currentRetry + 1
	status := "pending"
	if nextRetry >= maxEmbeddingRetries {
		status = "failed"
	}
	causeStr := cause.Error()
	return db.New(j.pool).MarkEmbeddingIndexFailed(ctx, db.MarkEmbeddingIndexFailedParams{
		ObjectType: objectType,
		ObjectID:   objectID,
		ModelID:    modelID,
		Status:     status,
		RetryCount: int32(nextRetry),
		LastError:  &causeStr,
	})
}
