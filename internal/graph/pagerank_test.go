package graph

import (
	"context"
	"strings"
	"testing"
)

func TestRunPageRank_NilPool_ReturnsError(t *testing.T) {
	if err := RunPageRank(context.Background(), nil); err == nil {
		t.Fatal("RunPageRank(nil) expected error, got nil")
	}
}

func TestRunPageRankSQL_UsesSchemaQualifiedFunction(t *testing.T) {
	if !strings.Contains(runPageRankSQL, "ag_catalog.age_pagerank(") {
		t.Fatalf("runPageRankSQL must use schema-qualified ag_catalog.age_pagerank: %q", runPageRankSQL)
	}
}
