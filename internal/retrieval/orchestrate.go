package retrieval

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
	graphpkg "github.com/simplyblock/postbrain/internal/graph"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/memory"
	"github.com/simplyblock/postbrain/internal/skills"
)

var graphAugmentationScopeSetFn = graphAugmentationScopeSet
var orchestrateMemoryRecallFn = orchestrateMemoryRecall
var orchestrateKnowledgeRecallFn = orchestrateKnowledgeRecall
var orchestrateSkillRecallFn = orchestrateSkillRecall

// OrchestrateDeps wires concrete stores used by OrchestrateRecall.
type OrchestrateDeps struct {
	Pool     *pgxpool.Pool
	MemStore *memory.Store
	KnwStore *knowledge.Store
	SklStore *skills.Store
	Svc      *embedding.EmbeddingService
}

// OrchestrateInput configures a cross-layer recall request.
type OrchestrateInput struct {
	Query              string
	ScopeID            uuid.UUID
	PrincipalID        uuid.UUID
	AuthorizedScopeIDs []uuid.UUID
	MemoryTypes        []string
	SearchMode         string
	AgentType          string
	Limit              int
	MinScore           float64
	GraphDepth         int
	ActiveLayers       map[Layer]bool
	Workdir            string
	StrictScope        bool
}

// OrchestrateRecall performs shared multi-layer recall and optional graph augmentation.
func OrchestrateRecall(ctx context.Context, deps OrchestrateDeps, input OrchestrateInput) ([]*Result, error) {
	if input.Limit <= 0 {
		input.Limit = 10
	}
	if input.SearchMode == "" {
		input.SearchMode = "hybrid"
	}
	if input.ActiveLayers == nil {
		input.ActiveLayers = map[Layer]bool{
			LayerMemory:    true,
			LayerKnowledge: true,
			LayerSkill:     true,
		}
	}

	var allResults []*Result

	if input.ActiveLayers[LayerMemory] && deps.MemStore != nil {
		mems, err := orchestrateMemoryRecallFn(ctx, deps, input)
		if err != nil {
			return nil, fmt.Errorf("memory recall failed: %w", err)
		}
		for _, m := range mems {
			r := &Result{
				Layer:      LayerMemory,
				ID:         m.Memory.ID,
				Score:      m.Score,
				Content:    m.Memory.Content,
				MemoryType: m.Memory.MemoryType,
				Importance: m.Memory.Importance,
				CreatedAt:  m.Memory.CreatedAt,
			}
			if m.Memory.SourceRef != nil {
				r.SourceRef = *m.Memory.SourceRef
			}
			if m.Memory.PromotedTo != nil {
				r.PromotedTo = m.Memory.PromotedTo
			}
			allResults = append(allResults, r)
		}
	}

	if input.ActiveLayers[LayerKnowledge] && deps.KnwStore != nil && deps.Pool != nil {
		arts, err := orchestrateKnowledgeRecallFn(ctx, deps, input)
		if err != nil {
			return nil, fmt.Errorf("knowledge recall failed: %w", err)
		}
		for _, a := range arts {
			aid := a.Artifact.ID
			go func() { _ = db.IncrementArtifactAccess(context.Background(), deps.Pool, aid) }()
			r := &Result{
				Layer:         LayerKnowledge,
				ID:            a.Artifact.ID,
				Score:         a.Score,
				Title:         a.Artifact.Title,
				KnowledgeType: a.Artifact.KnowledgeType,
				ArtifactKind:  a.Artifact.ArtifactKind,
				Visibility:    a.Artifact.Visibility,
				Status:        a.Artifact.Status,
				Endorsements:  int(a.Artifact.EndorsementCount),
			}
			if a.Artifact.Summary != nil && *a.Artifact.Summary != "" {
				r.Content = *a.Artifact.Summary
				r.Summary = *a.Artifact.Summary
				r.FullContentAvailable = true
			} else {
				r.Content = a.Artifact.Content
			}
			allResults = append(allResults, r)
		}
	}

	if input.ActiveLayers[LayerSkill] && deps.Pool != nil && deps.Svc != nil {
		skls, err := orchestrateSkillRecallFn(ctx, deps, input)
		if err != nil {
			return nil, fmt.Errorf("skill recall failed: %w", err)
		}
		for _, sk := range skls {
			allResults = append(allResults, &Result{
				Layer:           LayerSkill,
				ID:              sk.Skill.ID,
				Score:           sk.Score,
				Slug:            sk.Skill.Slug,
				Name:            sk.Skill.Name,
				Description:     sk.Skill.Description,
				AgentTypes:      sk.Skill.AgentTypes,
				InvocationCount: int(sk.Skill.InvocationCount),
				Installed:       sk.Installed,
			})
		}
	}

	merged := Merge(allResults, input.Limit, input.MinScore)
	if input.GraphDepth <= 0 || input.ScopeID == uuid.Nil || deps.Pool == nil {
		return merged, nil
	}
	hasSourceRef := false
	for _, r := range merged {
		if r.SourceRef != "" {
			hasSourceRef = true
			break
		}
	}
	if !hasSourceRef {
		return merged, nil
	}
	allowedGraphScopes := graphAugmentationScopeSetFn(ctx, deps.Pool, input.ScopeID, input.PrincipalID, input.AuthorizedScopeIDs)

	seen := make(map[uuid.UUID]bool, len(merged))
	for _, r := range merged {
		seen[r.ID] = true
	}
	var graphExtra []*Result
	for _, r := range merged {
		if r.SourceRef == "" {
			continue
		}
		symbol := trimSourceRefLine(r.SourceRef)
		entity, resolveErr := graphpkg.ResolveSymbol(ctx, deps.Pool, input.ScopeID, symbol)
		if resolveErr != nil || entity == nil {
			continue
		}
		neighbours, neighbourErr := graphpkg.NeighboursForEntity(ctx, deps.Pool, input.ScopeID, entity.ID)
		if neighbourErr != nil {
			continue
		}
		for _, nb := range neighbours {
			mems, memErr := db.ListMemoriesForEntity(ctx, deps.Pool, nb.Entity.ID, 3)
			if memErr != nil {
				continue
			}
			for _, m := range mems {
				if _, ok := allowedGraphScopes[m.ScopeID]; !ok {
					continue
				}
				if seen[m.ID] {
					continue
				}
				seen[m.ID] = true
				gr := &Result{
					Layer:        LayerMemory,
					ID:           m.ID,
					Score:        r.Score * nb.Confidence * 0.6,
					Content:      m.Content,
					MemoryType:   m.MemoryType,
					Importance:   m.Importance,
					CreatedAt:    m.CreatedAt,
					GraphContext: true,
				}
				if m.SourceRef != nil {
					gr.SourceRef = *m.SourceRef
				}
				graphExtra = append(graphExtra, gr)
			}
		}
	}
	return append(merged, graphExtra...), nil
}

func orchestrateMemoryRecall(ctx context.Context, deps OrchestrateDeps, input OrchestrateInput) ([]*memory.MemoryResult, error) {
	return deps.MemStore.Recall(ctx, memory.RecallInput{
		Query:              input.Query,
		ScopeID:            input.ScopeID,
		PrincipalID:        input.PrincipalID,
		AuthorizedScopeIDs: input.AuthorizedScopeIDs,
		MemoryTypes:        input.MemoryTypes,
		SearchMode:         input.SearchMode,
		Limit:              input.Limit * 2,
		MinScore:           input.MinScore,
		StrictScope:        input.StrictScope,
	})
}

func orchestrateKnowledgeRecall(ctx context.Context, deps OrchestrateDeps, input OrchestrateInput) ([]*knowledge.ArtifactResult, error) {
	return deps.KnwStore.Recall(ctx, deps.Pool, knowledge.RecallInput{
		Query:    input.Query,
		ScopeID:  input.ScopeID,
		Limit:    input.Limit * 2,
		MinScore: input.MinScore,
	})
}

func orchestrateSkillRecall(ctx context.Context, deps OrchestrateDeps, input OrchestrateInput) ([]*skills.SkillResult, error) {
	sklStore := deps.SklStore
	if sklStore == nil {
		sklStore = skills.NewStore(deps.Pool, nil)
	}
	return sklStore.Recall(ctx, deps.Svc, skills.RecallInput{
		Query:     input.Query,
		ScopeIDs:  []uuid.UUID{input.ScopeID},
		AgentType: input.AgentType,
		Limit:     input.Limit * 2,
		MinScore:  input.MinScore,
		Workdir:   input.Workdir,
	})
}

func graphAugmentationScopeSet(
	ctx context.Context,
	pool *pgxpool.Pool,
	scopeID, principalID uuid.UUID,
	authorizedScopeIDs []uuid.UUID,
) map[uuid.UUID]struct{} {
	out := make(map[uuid.UUID]struct{})
	fanout, err := memory.FanOutScopeIDs(ctx, pool, scopeID, principalID, 0, false)
	if err != nil || len(fanout) == 0 {
		out[scopeID] = struct{}{}
	} else {
		for _, id := range fanout {
			out[id] = struct{}{}
		}
	}
	if len(authorizedScopeIDs) == 0 {
		return out
	}
	authSet := make(map[uuid.UUID]struct{}, len(authorizedScopeIDs))
	for _, id := range authorizedScopeIDs {
		authSet[id] = struct{}{}
	}
	filtered := make(map[uuid.UUID]struct{}, len(out))
	for id := range out {
		if _, ok := authSet[id]; ok {
			filtered[id] = struct{}{}
		}
	}
	if len(filtered) == 0 {
		// Be conservative: when intersection is empty, keep selected scope only.
		return map[uuid.UUID]struct{}{scopeID: {}}
	}
	return filtered
}

func trimSourceRefLine(sourceRef string) string {
	symbol := sourceRef
	if len(symbol) > 5 && symbol[:5] == "file:" {
		symbol = symbol[5:]
		// Strip trailing :line.
		for i := len(symbol) - 1; i >= 0; i-- {
			if symbol[i] == ':' {
				if _, err := fmt.Sscanf(symbol[i+1:], "%d", new(int)); err == nil {
					symbol = symbol[:i]
				}
				break
			}
		}
	}
	return symbol
}
