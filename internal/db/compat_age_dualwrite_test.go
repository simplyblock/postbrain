package db

import (
	"errors"
	"testing"
)

func TestBestEffortAGEDualWriteError_NilError(t *testing.T) {
	if err := bestEffortAGEDualWriteError("entity", nil); err != nil {
		t.Fatalf("nil input error should remain nil, got %v", err)
	}
}

func TestBestEffortAGEDualWriteError_SwallowsError(t *testing.T) {
	in := errors.New("age permission denied")
	if err := bestEffortAGEDualWriteError("relation", in); err != nil {
		t.Fatalf("best-effort handler should swallow age dual-write errors, got %v", err)
	}
}
