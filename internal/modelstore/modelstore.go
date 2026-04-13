// Package modelstore provides DB-backed stores for resolving AI model metadata.
// It is intentionally separate from internal/embedding so that generation/summary
// consumers can depend on model lookup without importing embedding provider code.
package modelstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/simplyblock/postbrain/internal/db"
)

// ModelConfig contains the provider/runtime settings for one AI model.
type ModelConfig struct {
	ID             uuid.UUID
	Provider       string
	ProviderConfig string
	ServiceURL     string
	ProviderModel  string
	Dimensions     int
}

// EmbeddingModelStore resolves embedding model metadata.
type EmbeddingModelStore interface {
	GetModelConfig(ctx context.Context, modelID uuid.UUID) (*ModelConfig, error)
	ActiveModelIDByContentType(ctx context.Context, contentType string) (*uuid.UUID, error)
}

// GenerationModelStore resolves generation/summary model metadata.
type GenerationModelStore interface {
	GetModelConfig(ctx context.Context, modelID uuid.UUID) (*ModelConfig, error)
	ActiveGenerationModelIDByContentType(ctx context.Context, contentType string) (*uuid.UUID, error)
}

// DBModelStore is a DB-backed implementation of both EmbeddingModelStore and
// GenerationModelStore.
type DBModelStore struct {
	q db.DBTX
}

// NewDBModelStore constructs a DB-backed model config store.
func NewDBModelStore(q db.DBTX) *DBModelStore {
	return &DBModelStore{q: q}
}

// NewEmbeddingModelStore constructs a DB-backed EmbeddingModelStore.
func NewEmbeddingModelStore(q db.DBTX) EmbeddingModelStore {
	return NewDBModelStore(q)
}

// NewGenerationModelStore constructs a DB-backed GenerationModelStore.
func NewGenerationModelStore(q db.DBTX) GenerationModelStore {
	return NewDBModelStore(q)
}

// GetModelConfig loads one model's runtime configuration by ID.
func (s *DBModelStore) GetModelConfig(ctx context.Context, modelID uuid.UUID) (*ModelConfig, error) {
	if s == nil || s.q == nil {
		return nil, fmt.Errorf("model store: queryer is not configured")
	}

	row, err := db.New(s.q).GetAIModelRuntimeConfigByID(ctx, modelID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("model store: query model %s: %w", modelID, err)
	}

	cfg := &ModelConfig{
		ID:             modelID,
		Dimensions:     int(row.Dimensions),
		ProviderConfig: row.ProviderConfig,
	}
	if row.Provider != nil {
		cfg.Provider = *row.Provider
	}
	if row.ServiceUrl != nil {
		cfg.ServiceURL = *row.ServiceUrl
	}
	if row.ProviderModel != nil {
		cfg.ProviderModel = *row.ProviderModel
	}
	return cfg, nil
}

// ActiveModelIDByTypeAndContent returns the active model ID for one model/content type pair.
// Returns (nil, nil) when no active model is registered.
func (s *DBModelStore) ActiveModelIDByTypeAndContent(ctx context.Context, modelType, contentType string) (*uuid.UUID, error) {
	if s == nil || s.q == nil {
		return nil, fmt.Errorf("model store: queryer is not configured")
	}

	id, err := db.New(s.q).GetActiveAIModelIDByTypeAndContent(ctx, db.GetActiveAIModelIDByTypeAndContentParams{
		ModelType:   modelType,
		ContentType: contentType,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("model store: query active %s/%s model: %w", modelType, contentType, err)
	}
	return &id, nil
}

// ActiveModelIDByContentType returns the active embedding model ID for one content type.
// Returns (nil, nil) when no active model is registered.
func (s *DBModelStore) ActiveModelIDByContentType(ctx context.Context, contentType string) (*uuid.UUID, error) {
	return s.ActiveModelIDByTypeAndContent(ctx, "embedding", contentType)
}

// ActiveGenerationModelIDByContentType returns the active generation model ID for one content type.
// Returns (nil, nil) when no active model is registered.
func (s *DBModelStore) ActiveGenerationModelIDByContentType(ctx context.Context, contentType string) (*uuid.UUID, error) {
	return s.ActiveModelIDByTypeAndContent(ctx, "generation", contentType)
}
