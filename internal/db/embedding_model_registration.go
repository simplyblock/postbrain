package db

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RegisterEmbeddingModelParams defines the registration contract for one model.
type RegisterEmbeddingModelParams struct {
	Slug           string
	Provider       string
	ServiceURL     string
	ProviderModel  string
	ProviderConfig string
	Dimensions     int
	ContentType    string
	Activate       bool
}

// RegisterEmbeddingModel registers or updates an embedding model in one transaction.
// It provisions the per-model table and populates pending embedding_index rows.
func RegisterEmbeddingModel(ctx context.Context, pool *pgxpool.Pool, params RegisterEmbeddingModelParams) (*EmbeddingModel, error) {
	if pool == nil {
		return nil, fmt.Errorf("db: register embedding model: nil pool")
	}
	if err := validateRegisterEmbeddingModelParams(params); err != nil {
		return nil, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("db: register embedding model: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if params.Activate {
		if err := deactivateEmbeddingModelsByType(ctx, tx, params.ContentType); err != nil {
			return nil, fmt.Errorf("db: register embedding model: deactivate existing models: %w", err)
		}
	}

	model, err := upsertEmbeddingModelTx(ctx, tx, params)
	if err != nil {
		return nil, fmt.Errorf("db: register embedding model: upsert model: %w", err)
	}

	tableName, err := ensureEmbeddingModelTable(ctx, tx, model.ID, params.Dimensions)
	if err != nil {
		return nil, fmt.Errorf("db: register embedding model: provision table: %w", err)
	}

	if err := markEmbeddingModelReady(ctx, tx, model.ID, tableName); err != nil {
		return nil, fmt.Errorf("db: register embedding model: set ready metadata: %w", err)
	}

	if err := seedEmbeddingIndexPendingRows(ctx, tx, model.ID); err != nil {
		return nil, fmt.Errorf("db: register embedding model: seed embedding index: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("db: register embedding model: commit tx: %w", err)
	}
	return model, nil
}

func validateRegisterEmbeddingModelParams(params RegisterEmbeddingModelParams) error {
	if params.Slug == "" {
		return fmt.Errorf("db: register embedding model: slug is required")
	}
	if params.Provider == "" {
		return fmt.Errorf("db: register embedding model: provider is required")
	}
	if params.ServiceURL == "" {
		return fmt.Errorf("db: register embedding model: service_url is required")
	}
	if params.ProviderModel == "" {
		return fmt.Errorf("db: register embedding model: provider_model is required")
	}
	if params.Dimensions <= 0 {
		return fmt.Errorf("db: register embedding model: dimensions must be > 0")
	}
	if params.ContentType != "text" && params.ContentType != "code" {
		return fmt.Errorf("db: register embedding model: invalid content_type %q", params.ContentType)
	}
	return nil
}

func deactivateEmbeddingModelsByType(ctx context.Context, tx DBTX, contentType string) error {
	_, err := tx.Exec(ctx, `
		UPDATE ai_models
		SET is_active = false
		WHERE model_type = 'embedding' AND content_type = $1
	`, contentType)
	return err
}

func upsertEmbeddingModelTx(ctx context.Context, tx DBTX, params RegisterEmbeddingModelParams) (*EmbeddingModel, error) {
	if params.ProviderConfig == "" {
		params.ProviderConfig = "default"
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO ai_models (slug, provider, service_url, provider_model, provider_config, dimensions, content_type, model_type, is_active, is_ready)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'embedding', $8, false)
		ON CONFLICT (slug) DO UPDATE SET
			provider = EXCLUDED.provider,
			service_url = EXCLUDED.service_url,
			provider_model = EXCLUDED.provider_model,
			provider_config = EXCLUDED.provider_config,
			dimensions = EXCLUDED.dimensions,
			content_type = EXCLUDED.content_type,
			model_type = EXCLUDED.model_type,
			is_active = EXCLUDED.is_active
		RETURNING id, slug, dimensions, content_type, is_active, description, created_at
	`, params.Slug, params.Provider, params.ServiceURL, params.ProviderModel, params.ProviderConfig, params.Dimensions, params.ContentType, params.Activate)

	model := &EmbeddingModel{}
	if err := row.Scan(
		&model.ID,
		&model.Slug,
		&model.Dimensions,
		&model.ContentType,
		&model.IsActive,
		&model.Description,
		&model.CreatedAt,
	); err != nil {
		return nil, err
	}
	return model, nil
}

func markEmbeddingModelReady(ctx context.Context, tx DBTX, modelID uuid.UUID, tableName string) error {
	_, err := tx.Exec(ctx, `
		UPDATE ai_models
		SET table_name = $2, is_ready = true
		WHERE id = $1
	`, modelID, tableName)
	return err
}

func seedEmbeddingIndexPendingRows(ctx context.Context, tx DBTX, modelID uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO embedding_index (object_type, object_id, model_id, status)
		SELECT 'memory'::text, id, $1::uuid, 'pending'::text FROM memories
		UNION ALL
		SELECT 'entity'::text, id, $1::uuid, 'pending'::text FROM entities
		UNION ALL
		SELECT 'knowledge_artifact'::text, id, $1::uuid, 'pending'::text FROM knowledge_artifacts
		UNION ALL
		SELECT 'skill'::text, id, $1::uuid, 'pending'::text FROM skills
		ON CONFLICT (object_type, object_id, model_id) DO NOTHING
	`, modelID)
	return err
}
