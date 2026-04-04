//go:build integration

package graph_test

import (
	"context"
	"errors"
	"testing"

	"github.com/simplyblock/postbrain/internal/graph"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestRunPageRank_WithoutAGE_ReturnsUnavailable(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	if err := graph.RunPageRank(ctx, pool); !errors.Is(err, graph.ErrAGEUnavailable) {
		t.Fatalf("RunPageRank without AGE: err=%v, want ErrAGEUnavailable", err)
	}
}
