package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type modelMetadata struct {
	tableName   *string
	dimensions  int
	contentType string
	isReady     bool
}

type modelTableMetadata struct {
	tableName  string
	dimensions int
}

func lookupModelMetadata(ctx context.Context, q DBTX, modelID uuid.UUID) (*modelMetadata, error) {
	var out modelMetadata
	err := q.QueryRow(ctx, `
		SELECT table_name, dimensions, content_type, is_ready
		FROM ai_models
		WHERE id = $1 AND model_type = 'embedding'
	`, modelID).Scan(&out.tableName, &out.dimensions, &out.contentType, &out.isReady)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("db: embedding model %s not found", modelID)
		}
		return nil, fmt.Errorf("db: lookup embedding model metadata: %w", err)
	}
	return &out, nil
}

func lookupReadyModelTableMetadata(ctx context.Context, q DBTX, modelID uuid.UUID) (*modelTableMetadata, error) {
	meta, err := lookupModelMetadata(ctx, q, modelID)
	if err != nil {
		return nil, err
	}
	if !meta.isReady || meta.tableName == nil || strings.TrimSpace(*meta.tableName) == "" {
		return nil, fmt.Errorf("db: embedding model %s is not ready", modelID)
	}
	if !isSafeTableName(*meta.tableName) {
		return nil, fmt.Errorf("db: unsafe embedding table name %q", *meta.tableName)
	}
	return &modelTableMetadata{tableName: *meta.tableName, dimensions: meta.dimensions}, nil
}
