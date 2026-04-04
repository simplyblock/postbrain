package graph

import (
	"context"
	"testing"
)

func TestRunPageRank_NilPool_ReturnsError(t *testing.T) {
	if err := RunPageRank(context.Background(), nil); err == nil {
		t.Fatal("RunPageRank(nil) expected error, got nil")
	}
}
