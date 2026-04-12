package embedding

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/simplyblock/postbrain/internal/db"
)

// DBModelStore resolves model metadata from ai_models.
type DBModelStore struct {
	q db.DBTX
}

// NewDBModelStore constructs a DB-backed model config store.
func NewDBModelStore(q db.DBTX) *DBModelStore {
	return &DBModelStore{q: q}
}

// GetModelConfig loads one model's runtime configuration by ID.
func (s *DBModelStore) GetModelConfig(ctx context.Context, modelID uuid.UUID) (*ModelConfig, error) {
	if s == nil || s.q == nil {
		return nil, fmt.Errorf("embedding model store: queryer is not configured")
	}

	row, err := db.New(s.q).GetAIModelRuntimeConfigByID(ctx, modelID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("embedding model store: query model %s: %w", modelID, err)
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
		return nil, fmt.Errorf("embedding model store: queryer is not configured")
	}

	id, err := db.New(s.q).GetActiveAIModelIDByTypeAndContent(ctx, db.GetActiveAIModelIDByTypeAndContentParams{
		ModelType:   modelType,
		ContentType: contentType,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("embedding model store: query active %s/%s model: %w", modelType, contentType, err)
	}
	return &id, nil
}

// ActiveModelIDByContentType returns the active embedding model ID for one content type.
// Returns (nil, nil) when no active model is registered.
func (s *DBModelStore) ActiveModelIDByContentType(ctx context.Context, contentType string) (*uuid.UUID, error) {
	return s.ActiveModelIDByTypeAndContent(ctx, "embedding", contentType)
}
