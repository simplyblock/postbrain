package db

import (
	"context"

	"github.com/google/uuid"
)

type embeddingUpserter interface {
	UpsertEmbedding(ctx context.Context, in UpsertEmbeddingInput) error
}

// UpsertEmbeddingIfPresent upserts into model-backed embedding tables when the
// repository, model ID, and vector are all present. Missing values are a no-op.
func UpsertEmbeddingIfPresent(
	ctx context.Context,
	upserter embeddingUpserter,
	objectType string,
	objectID, scopeID uuid.UUID,
	embeddingVec []float32,
	modelID *uuid.UUID,
) error {
	if upserter == nil || modelID == nil || len(embeddingVec) == 0 {
		return nil
	}
	return upserter.UpsertEmbedding(ctx, UpsertEmbeddingInput{
		ObjectType: objectType,
		ObjectID:   objectID,
		ScopeID:    scopeID,
		ModelID:    *modelID,
		Embedding:  embeddingVec,
	})
}
