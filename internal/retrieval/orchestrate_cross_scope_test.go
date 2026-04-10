package retrieval

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/memory"
	"github.com/simplyblock/postbrain/internal/skills"
)

func TestOrchestrateCrossScopeContext_EnforcesStrictScope(t *testing.T) {
	origMem := orchestrateMemoryRecallFn
	origKnw := orchestrateKnowledgeRecallFn
	origSkill := orchestrateSkillRecallFn
	defer func() {
		orchestrateMemoryRecallFn = origMem
		orchestrateKnowledgeRecallFn = origKnw
		orchestrateSkillRecallFn = origSkill
	}()

	called := false
	orchestrateMemoryRecallFn = func(_ context.Context, _ OrchestrateDeps, input OrchestrateInput) ([]*memory.MemoryResult, error) {
		called = true
		if !input.StrictScope {
			t.Fatal("expected StrictScope=true")
		}
		return nil, nil
	}
	orchestrateKnowledgeRecallFn = func(_ context.Context, _ OrchestrateDeps, _ OrchestrateInput) ([]*knowledge.ArtifactResult, error) {
		return nil, nil
	}
	orchestrateSkillRecallFn = func(_ context.Context, _ OrchestrateDeps, _ OrchestrateInput) ([]*skills.SkillResult, error) {
		return nil, nil
	}

	_, err := OrchestrateCrossScopeContext(context.Background(), OrchestrateDeps{
		MemStore: &memory.Store{},
	}, OrchestrateInput{
		Query:      "q",
		ScopeID:    uuid.New(),
		SearchMode: "hybrid",
		Limit:      5,
		ActiveLayers: map[Layer]bool{
			LayerMemory:    true,
			LayerKnowledge: false,
			LayerSkill:     false,
		},
	})
	if err != nil {
		t.Fatalf("OrchestrateCrossScopeContext returned error: %v", err)
	}
	if !called {
		t.Fatal("expected memory orchestration to be called")
	}
}

func TestOrchestrateCrossScopeContext_DefaultLayersDisableSkill(t *testing.T) {
	origMem := orchestrateMemoryRecallFn
	origKnw := orchestrateKnowledgeRecallFn
	origSkill := orchestrateSkillRecallFn
	defer func() {
		orchestrateMemoryRecallFn = origMem
		orchestrateKnowledgeRecallFn = origKnw
		orchestrateSkillRecallFn = origSkill
	}()

	memCalled := false
	knwCalled := false
	skillCalled := false

	orchestrateMemoryRecallFn = func(_ context.Context, _ OrchestrateDeps, _ OrchestrateInput) ([]*memory.MemoryResult, error) {
		memCalled = true
		return nil, nil
	}
	orchestrateKnowledgeRecallFn = func(_ context.Context, _ OrchestrateDeps, _ OrchestrateInput) ([]*knowledge.ArtifactResult, error) {
		knwCalled = true
		return nil, nil
	}
	orchestrateSkillRecallFn = func(_ context.Context, _ OrchestrateDeps, _ OrchestrateInput) ([]*skills.SkillResult, error) {
		skillCalled = true
		return nil, nil
	}

	_, err := OrchestrateCrossScopeContext(context.Background(), OrchestrateDeps{
		Pool:     &pgxpool.Pool{},
		MemStore: &memory.Store{},
		KnwStore: &knowledge.Store{},
		SklStore: &skills.Store{},
	}, OrchestrateInput{
		Query:      "q",
		ScopeID:    uuid.New(),
		SearchMode: "hybrid",
		Limit:      5,
	})
	if err != nil {
		t.Fatalf("OrchestrateCrossScopeContext returned error: %v", err)
	}
	if !memCalled {
		t.Fatal("expected memory orchestration to be called")
	}
	if !knwCalled {
		t.Fatal("expected knowledge orchestration to be called")
	}
	if skillCalled {
		t.Fatal("expected skill orchestration to be disabled by default")
	}
}
