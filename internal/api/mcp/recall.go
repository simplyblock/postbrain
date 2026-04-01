package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	graphpkg "github.com/simplyblock/postbrain/internal/graph"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/memory"
	"github.com/simplyblock/postbrain/internal/retrieval"
	"github.com/simplyblock/postbrain/internal/skills"
)

// handleRecall retrieves memories and knowledge relevant to a query.
func (s *Server) handleRecall(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	query, _ := args["query"].(string)
	if query == "" {
		return mcpgo.NewToolResultError("recall: 'query' is required"), nil
	}
	scopeStr, _ := args["scope"].(string)

	limit := 10
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	minScore := 0.0
	if v, ok := args["min_score"].(float64); ok {
		minScore = v
	}

	searchMode := "hybrid"
	if v, ok := args["search_mode"].(string); ok && v != "" {
		searchMode = v
	}

	agentType := ""
	if v, ok := args["agent_type"].(string); ok {
		agentType = v
	}

	graphDepth := 0
	if v, ok := args["graph_depth"].(float64); ok && v > 0 {
		graphDepth = int(v)
		if graphDepth > 2 {
			graphDepth = 2 // cap to avoid fanout explosion
		}
	}

	var memoryTypes []string
	if v, ok := args["memory_types"].([]any); ok {
		for _, mt := range v {
			if ms, ok := mt.(string); ok {
				memoryTypes = append(memoryTypes, ms)
			}
		}
	}

	// Parse layers (default: all three).
	activeLayers := map[retrieval.Layer]bool{
		retrieval.LayerMemory:    true,
		retrieval.LayerKnowledge: true,
		retrieval.LayerSkill:     true,
	}
	if v, ok := args["layers"].([]any); ok && len(v) > 0 {
		activeLayers = map[retrieval.Layer]bool{}
		for _, l := range v {
			if ls, ok := l.(string); ok {
				activeLayers[retrieval.Layer(ls)] = true
			}
		}
	}

	if s.pool == nil {
		return mcpgo.NewToolResultError("recall: server not configured (no database connection)"), nil
	}

	// Resolve scope.
	var scopeID uuid.UUID
	if scopeStr != "" {
		kind, externalID, err := parseScopeString(scopeStr)
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("recall: invalid scope: %v", err)), nil
		}
		scope, err := db.GetScopeByExternalID(ctx, s.pool, kind, externalID)
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("recall: scope lookup: %v", err)), nil
		}
		if scope == nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("recall: scope '%s' not found", scopeStr)), nil
		}
		scopeID = scope.ID
	}

	principalID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	var allResults []*retrieval.Result

	// Memory layer.
	if activeLayers[retrieval.LayerMemory] && s.memStore != nil {
		mems, err := s.memStore.Recall(ctx, memory.RecallInput{
			Query:       query,
			ScopeID:     scopeID,
			PrincipalID: principalID,
			MemoryTypes: memoryTypes,
			SearchMode:  searchMode,
			Limit:       limit * 2,
			MinScore:    minScore,
		})
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("recall: memory recall failed: %v", err)), nil
		}
		for _, m := range mems {
			r := &retrieval.Result{
				Layer:      retrieval.LayerMemory,
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

	// Knowledge layer.
	if activeLayers[retrieval.LayerKnowledge] && s.knwStore != nil {
		arts, err := s.knwStore.Recall(ctx, s.pool, knowledge.RecallInput{
			Query:    query,
			ScopeID:  scopeID,
			Limit:    limit * 2,
			MinScore: minScore,
		})
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("recall: knowledge recall failed: %v", err)), nil
		}
		for _, a := range arts {
			aid := a.Artifact.ID
			go func() { _ = db.IncrementArtifactAccess(context.Background(), s.pool, aid) }()
			r := &retrieval.Result{
				Layer:         retrieval.LayerKnowledge,
				ID:            a.Artifact.ID,
				Score:         a.Score,
				Title:         a.Artifact.Title,
				KnowledgeType: a.Artifact.KnowledgeType,
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

	// Skill layer.
	if activeLayers[retrieval.LayerSkill] && s.sklStore != nil && s.svc != nil {
		skls, err := s.sklStore.Recall(ctx, s.svc, skills.RecallInput{
			Query:     query,
			ScopeIDs:  []uuid.UUID{scopeID},
			AgentType: agentType,
			Limit:     limit * 2,
			MinScore:  minScore,
		})
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("recall: skill recall failed: %v", err)), nil
		}
		for _, sk := range skls {
			allResults = append(allResults, &retrieval.Result{
				Layer:           retrieval.LayerSkill,
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

	merged := retrieval.Merge(allResults, limit, minScore)

	// Graph-augmented recall: for each code memory, fetch graph neighbours and
	// append memories linked to those neighbours (deduplicated).
	if graphDepth > 0 && scopeID != uuid.Nil && s.pool != nil {
		seen := make(map[uuid.UUID]bool, len(merged))
		for _, r := range merged {
			seen[r.ID] = true
		}
		var graphExtra []*retrieval.Result
		for _, r := range merged {
			if r.SourceRef == "" {
				continue
			}
			// Resolve the source file/symbol to an entity.
			symbol := r.SourceRef
			if len(symbol) > 5 && symbol[:5] == "file:" {
				symbol = symbol[5:]
				// Strip trailing :line.
				for i := len(symbol) - 1; i >= 0; i-- {
					if symbol[i] == ':' {
						if _, scanErr := fmt.Sscanf(symbol[i+1:], "%d", new(int)); scanErr == nil {
							symbol = symbol[:i]
						}
						break
					}
				}
			}
			entity, resolveErr := graphpkg.ResolveSymbol(ctx, s.pool, scopeID, symbol)
			if resolveErr != nil || entity == nil {
				continue
			}
			neighbours, neighbourErr := graphpkg.NeighboursForEntity(ctx, s.pool, scopeID, entity.ID)
			if neighbourErr != nil {
				continue
			}
			for _, nb := range neighbours {
				mems, memErr := db.ListMemoriesForEntity(ctx, s.pool, nb.Entity.ID, 3)
				if memErr != nil {
					continue
				}
				for _, m := range mems {
					if seen[m.ID] {
						continue
					}
					seen[m.ID] = true
					gr := &retrieval.Result{
						Layer:        retrieval.LayerMemory,
						ID:           m.ID,
						Score:        r.Score * nb.Confidence * 0.6, // discounted graph score
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
		merged = append(merged, graphExtra...)
	}

	type resultJSON struct {
		Layer                string   `json:"layer"`
		ID                   string   `json:"id"`
		Score                float64  `json:"score"`
		Content              string   `json:"content,omitempty"`
		Title                string   `json:"title,omitempty"`
		MemoryType           string   `json:"memory_type,omitempty"`
		KnowledgeType        string   `json:"knowledge_type,omitempty"`
		SourceRef            string   `json:"source_ref,omitempty"`
		Visibility           string   `json:"visibility,omitempty"`
		Status               string   `json:"status,omitempty"`
		Endorsements         int      `json:"endorsements,omitempty"`
		FullContentAvailable bool     `json:"full_content_available,omitempty"`
		Slug                 string   `json:"slug,omitempty"`
		Name                 string   `json:"name,omitempty"`
		Description          string   `json:"description,omitempty"`
		AgentTypes           []string `json:"agent_types,omitempty"`
		InvocationCount      int      `json:"invocation_count,omitempty"`
		Installed            bool     `json:"installed,omitempty"`
		GraphContext         bool     `json:"graph_context,omitempty"`
	}

	out := make([]resultJSON, 0, len(merged))
	for _, r := range merged {
		out = append(out, resultJSON{
			Layer:                string(r.Layer),
			ID:                   r.ID.String(),
			Score:                r.Score,
			Content:              r.Content,
			Title:                r.Title,
			MemoryType:           r.MemoryType,
			KnowledgeType:        r.KnowledgeType,
			SourceRef:            r.SourceRef,
			Visibility:           r.Visibility,
			Status:               r.Status,
			Endorsements:         r.Endorsements,
			FullContentAvailable: r.FullContentAvailable,
			Slug:                 r.Slug,
			Name:                 r.Name,
			Description:          r.Description,
			AgentTypes:           r.AgentTypes,
			InvocationCount:      r.InvocationCount,
			Installed:            r.Installed,
			GraphContext:         r.GraphContext,
		})
	}

	payload, _ := json.Marshal(map[string]any{"results": out})
	return mcpgo.NewToolResultText(string(payload)), nil
}
