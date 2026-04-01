package jobs

import (
	"context"
	"testing"
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

func TestContradictionJob_Signature(t *testing.T) {
	// Compile-time check.
	var _ = (*ContradictionJob)(nil).Run
}
