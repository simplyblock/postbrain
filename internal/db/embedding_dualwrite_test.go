package db

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type recordingEmbeddingUpserter struct {
	calls []UpsertEmbeddingInput
	err   error
}

func (r *recordingEmbeddingUpserter) UpsertEmbedding(_ context.Context, in UpsertEmbeddingInput) error {
	r.calls = append(r.calls, in)
	return r.err
}

func TestUpsertEmbeddingIfPresent_NoOpWhenMissingInputs(t *testing.T) {
	t.Parallel()

	objectID := uuid.New()
	scopeID := uuid.New()
	modelID := uuid.New()

	t.Run("nil upserter", func(t *testing.T) {
		t.Parallel()
		err := UpsertEmbeddingIfPresent(context.Background(), nil, "memory", objectID, scopeID, []float32{1}, &modelID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("nil model id", func(t *testing.T) {
		t.Parallel()
		upserter := &recordingEmbeddingUpserter{}
		err := UpsertEmbeddingIfPresent(context.Background(), upserter, "memory", objectID, scopeID, []float32{1}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(upserter.calls) != 0 {
			t.Fatalf("calls = %d, want 0", len(upserter.calls))
		}
	})

	t.Run("empty vector", func(t *testing.T) {
		t.Parallel()
		upserter := &recordingEmbeddingUpserter{}
		err := UpsertEmbeddingIfPresent(context.Background(), upserter, "memory", objectID, scopeID, nil, &modelID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(upserter.calls) != 0 {
			t.Fatalf("calls = %d, want 0", len(upserter.calls))
		}
	})
}

func TestUpsertEmbeddingIfPresent_UpsertsWithExpectedInput(t *testing.T) {
	t.Parallel()

	objectID := uuid.New()
	scopeID := uuid.New()
	modelID := uuid.New()
	upserter := &recordingEmbeddingUpserter{}

	err := UpsertEmbeddingIfPresent(context.Background(), upserter, "skill", objectID, scopeID, []float32{1, 2}, &modelID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(upserter.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(upserter.calls))
	}
	got := upserter.calls[0]
	if got.ObjectType != "skill" || got.ObjectID != objectID || got.ScopeID != scopeID || got.ModelID != modelID || len(got.Embedding) != 2 {
		t.Fatalf("unexpected upsert input: %#v", got)
	}
}

func TestUpsertEmbeddingIfPresent_PropagatesError(t *testing.T) {
	t.Parallel()

	want := errors.New("upsert failed")
	modelID := uuid.New()
	upserter := &recordingEmbeddingUpserter{err: want}

	err := UpsertEmbeddingIfPresent(context.Background(), upserter, "memory", uuid.New(), uuid.New(), []float32{1}, &modelID)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
