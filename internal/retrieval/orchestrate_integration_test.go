//go:build integration

package retrieval

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/memory"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestOrchestrateRecall_GraphContextDoesNotLeakSiblingScopeMemories(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	suffix := uuid.NewString()

	_ = testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "orchestrate-leak-user-"+suffix)
	company := testhelper.CreateTestScope(t, pool, "company", "orchestrate-leak-company-"+suffix, nil, principal.ID)
	selectedProject := testhelper.CreateTestScope(t, pool, "project", "orchestrate-selected-"+suffix, &company.ID, principal.ID)
	siblingProject := testhelper.CreateTestScope(t, pool, "project", "orchestrate-sibling-"+suffix, &company.ID, principal.ID)

	memStore := memory.NewStore(pool, svc)

	sourceRef := "file:src/selected.go:10"
	selectedMemory, err := memStore.Create(ctx, memory.CreateInput{
		MemoryType: "semantic",
		ScopeID:    selectedProject.ID,
		AuthorID:   principal.ID,
		Content:    "ORCHESTRATE_SCOPE_LEAK query anchor",
		SourceRef:  &sourceRef,
	})
	if err != nil {
		t.Fatalf("create selected memory: %v", err)
	}

	neighborEntity, err := compat.UpsertEntity(ctx, pool, &db.Entity{
		ScopeID:    selectedProject.ID,
		EntityType: "function",
		Name:       "NeighborFn",
		Canonical:  "pkg.NeighborFn",
	})
	if err != nil {
		t.Fatalf("upsert neighbor entity: %v", err)
	}
	sourceEntity, err := compat.UpsertEntity(ctx, pool, &db.Entity{
		ScopeID:    selectedProject.ID,
		EntityType: "file",
		Name:       "selected.go",
		Canonical:  "src/selected.go",
	})
	if err != nil {
		t.Fatalf("upsert source entity: %v", err)
	}
	if _, err := compat.UpsertRelation(ctx, pool, &db.Relation{
		ScopeID:   selectedProject.ID,
		SubjectID: sourceEntity.ID,
		Predicate: "uses",
		ObjectID:  neighborEntity.ID,
	}); err != nil {
		t.Fatalf("upsert relation: %v", err)
	}

	siblingMemory, err := memStore.Create(ctx, memory.CreateInput{
		MemoryType: "semantic",
		ScopeID:    siblingProject.ID,
		AuthorID:   principal.ID,
		Content:    "SIBLING_SCOPE_GRAPH_LEAK_MARKER",
	})
	if err != nil {
		t.Fatalf("create sibling memory: %v", err)
	}
	if err := compat.LinkMemoryToEntity(ctx, pool, siblingMemory.MemoryID, neighborEntity.ID, "related"); err != nil {
		t.Fatalf("link sibling memory to selected neighbor entity: %v", err)
	}

	results, err := OrchestrateRecall(ctx, OrchestrateDeps{
		Pool:     pool,
		MemStore: memStore,
		Svc:      svc,
	}, OrchestrateInput{
		Query:       "ORCHESTRATE_SCOPE_LEAK",
		ScopeID:     selectedProject.ID,
		PrincipalID: principal.ID,
		AuthorizedScopeIDs: []uuid.UUID{
			company.ID,
			selectedProject.ID,
			siblingProject.ID,
		},
		SearchMode: "hybrid",
		Limit:      10,
		GraphDepth: 1,
		ActiveLayers: map[Layer]bool{
			LayerMemory: true,
		},
	})
	if err != nil {
		t.Fatalf("OrchestrateRecall: %v", err)
	}

	var foundSelected bool
	for _, r := range results {
		if r.ID == selectedMemory.MemoryID {
			foundSelected = true
		}
		if strings.Contains(r.Content, "SIBLING_SCOPE_GRAPH_LEAK_MARKER") {
			t.Fatalf("unexpected sibling-scope graph memory in results: %q", r.Content)
		}
	}
	if !foundSelected {
		t.Fatalf("selected scope memory %s missing from results", selectedMemory.MemoryID)
	}
}
