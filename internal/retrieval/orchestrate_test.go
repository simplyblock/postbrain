package retrieval

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOrchestrateRecall_DoesNotComputeGraphAugmentationScopesWithoutSourceRefs(t *testing.T) {
	t.Helper()

	original := graphAugmentationScopeSetFn
	defer func() {
		graphAugmentationScopeSetFn = original
	}()

	called := false
	graphAugmentationScopeSetFn = func(
		ctx context.Context,
		pool *pgxpool.Pool,
		scopeID, principalID uuid.UUID,
		authorizedScopeIDs []uuid.UUID,
	) map[uuid.UUID]struct{} {
		called = true
		return map[uuid.UUID]struct{}{scopeID: {}}
	}

	scopeID := uuid.New()
	principalID := uuid.New()
	results, err := OrchestrateRecall(context.Background(), OrchestrateDeps{
		Pool: &pgxpool.Pool{},
	}, OrchestrateInput{
		ScopeID:     scopeID,
		PrincipalID: principalID,
		GraphDepth:  1,
		Limit:       10,
		ActiveLayers: map[Layer]bool{
			LayerMemory:    false,
			LayerKnowledge: false,
			LayerSkill:     false,
		},
	})
	if err != nil {
		t.Fatalf("OrchestrateRecall returned error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results, got %d", len(results))
	}
	if called {
		t.Fatal("expected graph augmentation scope set to be computed lazily")
	}
}
