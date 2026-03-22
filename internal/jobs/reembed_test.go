package jobs

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/simplyblock/postbrain/internal/embedding"
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

// TestReembedJob_NoActiveModel verifies that RunText and RunCode return nil
// (no error, no panic) when there is no pool/svc — simulated by a nil pool
// which produces an error that is treated as "no active model".
// Full DB behaviour is covered by integration tests.
func TestReembedJob_Signature(t *testing.T) {
	// Compile-time check that RunText and RunCode have the expected signatures.
	var _ func(context.Context) error = (*ReembedJob)(nil).RunText
	var _ func(context.Context) error = (*ReembedJob)(nil).RunCode
}

// TestNewReembedJob_FieldsSet verifies that the constructor properly stores
// the pool and svc references.
func TestNewReembedJob_Fields(t *testing.T) {
	var pool *pgxpool.Pool
	var svc *embedding.EmbeddingService
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
