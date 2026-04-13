//go:build integration

package testhelper

import (
	"github.com/simplyblock/postbrain/internal/providers"
)

// testModelDims must match the vector dimension registered by CreateTestEmbeddingModel.
const testModelDims = 4

// NewMockEmbeddingService returns an EmbeddingService backed by a FakeEmbedder
// with 4 dimensions. Each call returns a deterministic vector, matching what
// CreateTestEmbeddingModel registers in the test database.
func NewMockEmbeddingService() *providers.EmbeddingService {
	return providers.NewServiceFromEmbedders(providers.NewFakeEmbedder(testModelDims), nil)
}

// NewDeterministicEmbeddingService is an alias for NewMockEmbeddingService.
// Identical content produces identical vectors (cosine distance = 0), so
// near-duplicate detection fires as expected in integration tests.
func NewDeterministicEmbeddingService() *providers.EmbeddingService {
	return NewMockEmbeddingService()
}
