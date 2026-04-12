package jobs

import (
	"context"
	"testing"

	"github.com/google/uuid"
	pgvector "github.com/pgvector/pgvector-go"

	"github.com/simplyblock/postbrain/internal/db"
)

func TestNoopClassifier_ReturnsConsistent(t *testing.T) {
	verdict, reasoning, err := noopClassifier(context.Background(), "artifact content", "memory content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verdict != "CONSISTENT" {
		t.Errorf("expected CONSISTENT, got %q", verdict)
	}
	if reasoning == "" {
		t.Error("expected non-empty reasoning")
	}
}

func TestContradictionJob_NilClassifier_UsesNoop(t *testing.T) {
	j := NewContradictionJob(nil, nil, nil)
	if j.classify == nil {
		t.Error("expected classify to be set to noopClassifier when nil is passed")
	}
	// Verify the assigned function behaves as noop.
	verdict, _, err := j.classify(context.Background(), "a", "b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verdict != "CONSISTENT" {
		t.Errorf("expected CONSISTENT from noop, got %q", verdict)
	}
}

func TestFilterByTopicSimilarity(t *testing.T) {
	t.Parallel()
	j := &ContradictionJob{}

	// Parse a unit vector via pgvector.
	makeVec := func(s string) *pgvector.Vector {
		var v pgvector.Vector
		if err := v.Scan(s); err != nil {
			t.Fatalf("scan vector %q: %v", s, err)
		}
		return &v
	}

	artifactVec := []float32{1, 0, 0, 0}

	// Identical vector → cosine sim = 1.0 > 0.6 → kept.
	similar := &db.Memory{ID: uuid.New(), Embedding: makeVec("[1,0,0,0]")}
	// Orthogonal vector → cosine sim = 0.0 → filtered.
	orthogonal := &db.Memory{ID: uuid.New(), Embedding: makeVec("[0,1,0,0]")}
	// Nil embedding → filtered (no vector to compare).
	nilEmb := &db.Memory{ID: uuid.New(), Embedding: nil}

	results := j.filterByTopicSimilarity(artifactVec, []*db.Memory{similar, orthogonal, nilEmb})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != similar.ID {
		t.Error("expected only the similar memory to pass the topic similarity filter")
	}
}
