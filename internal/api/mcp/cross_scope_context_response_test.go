package mcp

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/retrieval"
)

func TestAsCrossScopeResultJSON_IncludesMandatoryProvenanceKeys(t *testing.T) {
	scope := "project:acme/docs"
	now := time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC)
	results := []*retrieval.Result{
		{
			Layer:     retrieval.LayerMemory,
			ID:        uuid.New(),
			Score:     0.8,
			Content:   "sample",
			SourceRef: "file:README.md:1",
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	out := asCrossScopeResultJSON(scope, results)
	if len(out) != 1 {
		t.Fatalf("len(out)=%d, want 1", len(out))
	}
	item := out[0]
	required := []string{"scope", "layer", "id", "score", "source_ref", "created_at", "updated_at"}
	for _, k := range required {
		if _, ok := item[k]; !ok {
			t.Fatalf("missing required key %q", k)
		}
	}
	if got, _ := item["scope"].(string); got != scope {
		t.Fatalf("scope=%q, want %q", got, scope)
	}
}

func TestAsCrossScopeResultJSON_UsesExplicitNullForUnavailableOptionalProvenance(t *testing.T) {
	scope := "project:acme/docs"
	results := []*retrieval.Result{
		{
			Layer: retrieval.LayerKnowledge,
			ID:    uuid.New(),
			Score: 0.8,
		},
	}
	out := asCrossScopeResultJSON(scope, results)
	if len(out) != 1 {
		t.Fatalf("len(out)=%d, want 1", len(out))
	}
	item := out[0]
	if v, ok := item["source_ref"]; !ok || v != nil {
		t.Fatalf("source_ref=%v (present=%v), want nil", v, ok)
	}
	if v, ok := item["created_at"]; !ok || v != nil {
		t.Fatalf("created_at=%v (present=%v), want nil", v, ok)
	}
	if v, ok := item["updated_at"]; !ok || v != nil {
		t.Fatalf("updated_at=%v (present=%v), want nil", v, ok)
	}
}
