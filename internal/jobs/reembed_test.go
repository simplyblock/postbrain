package jobs

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/simplyblock/postbrain/internal/providers"
)

func TestNewReembedJob_DefaultBatchSize(t *testing.T) {
	j := NewReembedJob(nil, nil, 0)
	if j.batchSize != 64 {
		t.Errorf("expected default batchSize=64, got %d", j.batchSize)
	}
}

func TestNewReembedJob_CustomBatchSize(t *testing.T) {
	j := NewReembedJob(nil, nil, 32)
	if j.batchSize != 32 {
		t.Errorf("expected batchSize=32, got %d", j.batchSize)
	}
}

// TestNewReembedJob_FieldsSet verifies that the constructor properly stores
// the pool and svc references.
func TestNewReembedJob_Fields(t *testing.T) {
	var pool *pgxpool.Pool
	var svc *providers.EmbeddingService
	j := NewReembedJob(pool, svc, 16)
	if j.pool != pool {
		t.Error("expected pool to be stored")
	}
	if j.svc != svc {
		t.Error("expected svc to be stored")
	}
	if j.batchSize != 16 {
		t.Errorf("expected batchSize=16, got %d", j.batchSize)
	}
}
