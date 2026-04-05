package embedding

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type modelRowQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// DBModelStore resolves embedding model metadata from embedding_models.
type DBModelStore struct {
	q modelRowQueryer
}

// NewDBModelStore constructs a DB-backed model config store.
func NewDBModelStore(q modelRowQueryer) *DBModelStore {
	return &DBModelStore{q: q}
}

// GetModelConfig loads one model's runtime configuration by ID.
func (s *DBModelStore) GetModelConfig(ctx context.Context, modelID uuid.UUID) (*ModelConfig, error) {
	if s == nil || s.q == nil {
		return nil, fmt.Errorf("embedding model store: queryer is not configured")
	}

	var (
		provider      string
		serviceURL    *string
		providerModel string
		dimensions    int
	)
	err := s.q.QueryRow(ctx, `
		SELECT provider, service_url, provider_model, dimensions
		FROM embedding_models
		WHERE id = $1
	`, modelID).Scan(&provider, &serviceURL, &providerModel, &dimensions)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("embedding model store: query model %s: %w", modelID, err)
	}

	cfg := &ModelConfig{
		ID:            modelID,
		Provider:      provider,
		ProviderModel: providerModel,
		Dimensions:    dimensions,
	}
	if serviceURL != nil {
		cfg.ServiceURL = *serviceURL
	}
	return cfg, nil
}

// ActiveModelIDByContentType returns the active model ID for one content type.
// Returns (nil, nil) when no active model is registered.
func (s *DBModelStore) ActiveModelIDByContentType(ctx context.Context, contentType string) (*uuid.UUID, error) {
	if s == nil || s.q == nil {
		return nil, fmt.Errorf("embedding model store: queryer is not configured")
	}

	var id uuid.UUID
	err := s.q.QueryRow(ctx, `
		SELECT id
		FROM embedding_models
		WHERE is_active = true AND content_type = $1
		LIMIT 1
	`, contentType).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("embedding model store: query active %s model: %w", contentType, err)
	}
	return &id, nil
}
