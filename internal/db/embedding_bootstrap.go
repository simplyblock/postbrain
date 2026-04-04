package db

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BootstrapLegacyEmbeddingsStats reports copy progress for one model.
type BootstrapLegacyEmbeddingsStats struct {
	UpsertedRows int64
	IndexedRows  int64
}

// BootstrapLegacyEmbeddingsForModel copies legacy inline vectors into the
// model-specific embedding table and marks embedding_index rows as ready.
func BootstrapLegacyEmbeddingsForModel(ctx context.Context, pool *pgxpool.Pool, modelID uuid.UUID) (*BootstrapLegacyEmbeddingsStats, error) {
	if pool == nil {
		return nil, fmt.Errorf("db: bootstrap legacy embeddings: nil pool")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("db: bootstrap legacy embeddings: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var (
		dimensions int
		contentTyp string
	)
	modelMeta, err := lookupModelMetadata(ctx, tx, modelID)
	if err != nil {
		return nil, fmt.Errorf("db: bootstrap legacy embeddings: %w", err)
	}
	tableName := modelMeta.tableName
	dimensions = modelMeta.dimensions
	contentTyp = modelMeta.contentType

	name := ""
	if tableName != nil {
		name = strings.TrimSpace(*tableName)
	}
	if name == "" {
		name = EmbeddingTableName(modelID)
	}
	if !isSafeTableName(name) {
		return nil, fmt.Errorf("db: bootstrap legacy embeddings: unsafe table name %q", name)
	}

	if _, err := ensureEmbeddingModelTable(ctx, tx, modelID, dimensions); err != nil {
		return nil, fmt.Errorf("db: bootstrap legacy embeddings: ensure model table: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE embedding_models
		SET table_name = $2, is_ready = true
		WHERE id = $1
	`, modelID, name); err != nil {
		return nil, fmt.Errorf("db: bootstrap legacy embeddings: set model ready: %w", err)
	}

	stats := &BootstrapLegacyEmbeddingsStats{}
	if contentTyp == "code" {
		up, idx, err := bootstrapCodeMemoryEmbeddings(ctx, tx, name, modelID)
		if err != nil {
			return nil, err
		}
		slog.InfoContext(ctx, "db: bootstrap embeddings stage", "model_id", modelID, "content_type", contentTyp, "stage", "memory_code", "upserted", up, "indexed", idx)
		stats.UpsertedRows += up
		stats.IndexedRows += idx
	} else {
		for _, stage := range []struct {
			name string
			fn   func(context.Context, DBTX, string, uuid.UUID) (int64, int64, error)
		}{
			{name: "memory_text", fn: bootstrapTextMemoryEmbeddings},
			{name: "entity", fn: bootstrapEntityEmbeddings},
			{name: "knowledge_artifact", fn: bootstrapKnowledgeEmbeddings},
			{name: "skill", fn: bootstrapSkillEmbeddings},
		} {
			up, idx, err := stage.fn(ctx, tx, name, modelID)
			if err != nil {
				return nil, err
			}
			slog.InfoContext(ctx, "db: bootstrap embeddings stage", "model_id", modelID, "content_type", contentTyp, "stage", stage.name, "upserted", up, "indexed", idx)
			stats.UpsertedRows += up
			stats.IndexedRows += idx
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("db: bootstrap legacy embeddings: commit tx: %w", err)
	}
	slog.InfoContext(ctx, "db: bootstrap embeddings complete", "model_id", modelID, "content_type", contentTyp, "upserted_rows", stats.UpsertedRows, "indexed_rows", stats.IndexedRows)
	return stats, nil
}

func bootstrapTextMemoryEmbeddings(ctx context.Context, tx DBTX, tableName string, modelID uuid.UUID) (int64, int64, error) {
	upsertSQL := fmt.Sprintf(`
		INSERT INTO %s (object_type, object_id, scope_id, embedding)
		SELECT 'memory', m.id, m.scope_id, m.embedding
		FROM memories m
		WHERE m.embedding_model_id = $1 AND m.embedding IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM embedding_index ei
			WHERE ei.object_type = 'memory' AND ei.object_id = m.id AND ei.model_id = $1 AND ei.status = 'ready'
		  )
		ON CONFLICT (object_type, object_id)
		DO UPDATE SET scope_id = EXCLUDED.scope_id, embedding = EXCLUDED.embedding, updated_at = now()
	`, tableName)
	upTag, err := tx.Exec(ctx, upsertSQL, modelID)
	if err != nil {
		return 0, 0, fmt.Errorf("db: bootstrap legacy embeddings: upsert text memories: %w", err)
	}

	idxTag, err := tx.Exec(ctx, `
		INSERT INTO embedding_index (object_type, object_id, model_id, status, retry_count, last_error)
		SELECT 'memory', m.id, $1, 'ready', 0, NULL
		FROM memories m
		WHERE m.embedding_model_id = $1 AND m.embedding IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM embedding_index ei
			WHERE ei.object_type = 'memory' AND ei.object_id = m.id AND ei.model_id = $1 AND ei.status = 'ready'
		  )
		ON CONFLICT (object_type, object_id, model_id)
		DO UPDATE SET status = 'ready', retry_count = 0, last_error = NULL, updated_at = now()
	`, modelID)
	if err != nil {
		return 0, 0, fmt.Errorf("db: bootstrap legacy embeddings: index text memories: %w", err)
	}
	return upTag.RowsAffected(), idxTag.RowsAffected(), nil
}

func bootstrapCodeMemoryEmbeddings(ctx context.Context, tx DBTX, tableName string, modelID uuid.UUID) (int64, int64, error) {
	upsertSQL := fmt.Sprintf(`
		INSERT INTO %s (object_type, object_id, scope_id, embedding)
		SELECT 'memory', m.id, m.scope_id, m.embedding_code
		FROM memories m
		WHERE m.embedding_code_model_id = $1 AND m.embedding_code IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM embedding_index ei
			WHERE ei.object_type = 'memory' AND ei.object_id = m.id AND ei.model_id = $1 AND ei.status = 'ready'
		  )
		ON CONFLICT (object_type, object_id)
		DO UPDATE SET scope_id = EXCLUDED.scope_id, embedding = EXCLUDED.embedding, updated_at = now()
	`, tableName)
	upTag, err := tx.Exec(ctx, upsertSQL, modelID)
	if err != nil {
		return 0, 0, fmt.Errorf("db: bootstrap legacy embeddings: upsert code memories: %w", err)
	}

	idxTag, err := tx.Exec(ctx, `
		INSERT INTO embedding_index (object_type, object_id, model_id, status, retry_count, last_error)
		SELECT 'memory', m.id, $1, 'ready', 0, NULL
		FROM memories m
		WHERE m.embedding_code_model_id = $1 AND m.embedding_code IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM embedding_index ei
			WHERE ei.object_type = 'memory' AND ei.object_id = m.id AND ei.model_id = $1 AND ei.status = 'ready'
		  )
		ON CONFLICT (object_type, object_id, model_id)
		DO UPDATE SET status = 'ready', retry_count = 0, last_error = NULL, updated_at = now()
	`, modelID)
	if err != nil {
		return 0, 0, fmt.Errorf("db: bootstrap legacy embeddings: index code memories: %w", err)
	}
	return upTag.RowsAffected(), idxTag.RowsAffected(), nil
}

func bootstrapEntityEmbeddings(ctx context.Context, tx DBTX, tableName string, modelID uuid.UUID) (int64, int64, error) {
	upsertSQL := fmt.Sprintf(`
		INSERT INTO %s (object_type, object_id, scope_id, embedding)
		SELECT 'entity', e.id, e.scope_id, e.embedding
		FROM entities e
		WHERE e.embedding_model_id = $1 AND e.embedding IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM embedding_index ei
			WHERE ei.object_type = 'entity' AND ei.object_id = e.id AND ei.model_id = $1 AND ei.status = 'ready'
		  )
		ON CONFLICT (object_type, object_id)
		DO UPDATE SET scope_id = EXCLUDED.scope_id, embedding = EXCLUDED.embedding, updated_at = now()
	`, tableName)
	upTag, err := tx.Exec(ctx, upsertSQL, modelID)
	if err != nil {
		return 0, 0, fmt.Errorf("db: bootstrap legacy embeddings: upsert entities: %w", err)
	}

	idxTag, err := tx.Exec(ctx, `
		INSERT INTO embedding_index (object_type, object_id, model_id, status, retry_count, last_error)
		SELECT 'entity', e.id, $1, 'ready', 0, NULL
		FROM entities e
		WHERE e.embedding_model_id = $1 AND e.embedding IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM embedding_index ei
			WHERE ei.object_type = 'entity' AND ei.object_id = e.id AND ei.model_id = $1 AND ei.status = 'ready'
		  )
		ON CONFLICT (object_type, object_id, model_id)
		DO UPDATE SET status = 'ready', retry_count = 0, last_error = NULL, updated_at = now()
	`, modelID)
	if err != nil {
		return 0, 0, fmt.Errorf("db: bootstrap legacy embeddings: index entities: %w", err)
	}
	return upTag.RowsAffected(), idxTag.RowsAffected(), nil
}

func bootstrapKnowledgeEmbeddings(ctx context.Context, tx DBTX, tableName string, modelID uuid.UUID) (int64, int64, error) {
	upsertSQL := fmt.Sprintf(`
		INSERT INTO %s (object_type, object_id, scope_id, embedding)
		SELECT 'knowledge_artifact', k.id, k.owner_scope_id, k.embedding
		FROM knowledge_artifacts k
		WHERE k.embedding_model_id = $1 AND k.embedding IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM embedding_index ei
			WHERE ei.object_type = 'knowledge_artifact' AND ei.object_id = k.id AND ei.model_id = $1 AND ei.status = 'ready'
		  )
		ON CONFLICT (object_type, object_id)
		DO UPDATE SET scope_id = EXCLUDED.scope_id, embedding = EXCLUDED.embedding, updated_at = now()
	`, tableName)
	upTag, err := tx.Exec(ctx, upsertSQL, modelID)
	if err != nil {
		return 0, 0, fmt.Errorf("db: bootstrap legacy embeddings: upsert knowledge artifacts: %w", err)
	}

	idxTag, err := tx.Exec(ctx, `
		INSERT INTO embedding_index (object_type, object_id, model_id, status, retry_count, last_error)
		SELECT 'knowledge_artifact', k.id, $1, 'ready', 0, NULL
		FROM knowledge_artifacts k
		WHERE k.embedding_model_id = $1 AND k.embedding IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM embedding_index ei
			WHERE ei.object_type = 'knowledge_artifact' AND ei.object_id = k.id AND ei.model_id = $1 AND ei.status = 'ready'
		  )
		ON CONFLICT (object_type, object_id, model_id)
		DO UPDATE SET status = 'ready', retry_count = 0, last_error = NULL, updated_at = now()
	`, modelID)
	if err != nil {
		return 0, 0, fmt.Errorf("db: bootstrap legacy embeddings: index knowledge artifacts: %w", err)
	}
	return upTag.RowsAffected(), idxTag.RowsAffected(), nil
}

func bootstrapSkillEmbeddings(ctx context.Context, tx DBTX, tableName string, modelID uuid.UUID) (int64, int64, error) {
	upsertSQL := fmt.Sprintf(`
		INSERT INTO %s (object_type, object_id, scope_id, embedding)
		SELECT 'skill', s.id, s.scope_id, s.embedding
		FROM skills s
		WHERE s.embedding_model_id = $1 AND s.embedding IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM embedding_index ei
			WHERE ei.object_type = 'skill' AND ei.object_id = s.id AND ei.model_id = $1 AND ei.status = 'ready'
		  )
		ON CONFLICT (object_type, object_id)
		DO UPDATE SET scope_id = EXCLUDED.scope_id, embedding = EXCLUDED.embedding, updated_at = now()
	`, tableName)
	upTag, err := tx.Exec(ctx, upsertSQL, modelID)
	if err != nil {
		return 0, 0, fmt.Errorf("db: bootstrap legacy embeddings: upsert skills: %w", err)
	}

	idxTag, err := tx.Exec(ctx, `
		INSERT INTO embedding_index (object_type, object_id, model_id, status, retry_count, last_error)
		SELECT 'skill', s.id, $1, 'ready', 0, NULL
		FROM skills s
		WHERE s.embedding_model_id = $1 AND s.embedding IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM embedding_index ei
			WHERE ei.object_type = 'skill' AND ei.object_id = s.id AND ei.model_id = $1 AND ei.status = 'ready'
		  )
		ON CONFLICT (object_type, object_id, model_id)
		DO UPDATE SET status = 'ready', retry_count = 0, last_error = NULL, updated_at = now()
	`, modelID)
	if err != nil {
		return 0, 0, fmt.Errorf("db: bootstrap legacy embeddings: index skills: %w", err)
	}
	return upTag.RowsAffected(), idxTag.RowsAffected(), nil
}
