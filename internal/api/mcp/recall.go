package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/memory"
	"github.com/simplyblock/postbrain/internal/retrieval"
	"github.com/simplyblock/postbrain/internal/skills"
)

// handleRecall retrieves memories and knowledge relevant to a query.
func (s *Server) handleRecall(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	query, _ := args["query"].(string)
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
			if m.Memory.PromotedTo != (uuid.UUID{}) {
				promotedTo := m.Memory.PromotedTo
				r.PromotedTo = &promotedTo
			}
			allResults = append(allResults, r)
		}
	}

	// Knowledge layer.
	if activeLayers[retrieval.LayerKnowledge] && s.knwStore != nil {
		arts, err := s.knwStore.Recall(ctx, s.pool, knowledge.RecallInput{
			Query:    query,
			ScopeIDs: []uuid.UUID{scopeID},
			Limit:    limit * 2,
			MinScore: minScore,
		})
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("recall: knowledge recall failed: %v", err)), nil
		}
		for _, a := range arts {
			allResults = append(allResults, &retrieval.Result{
				Layer:         retrieval.LayerKnowledge,
				ID:            a.Artifact.ID,
				Score:         a.Score,
				Title:         a.Artifact.Title,
				Content:       a.Artifact.Content,
				KnowledgeType: a.Artifact.KnowledgeType,
				Visibility:    a.Artifact.Visibility,
				Status:        a.Artifact.Status,
				Endorsements:  int(a.Artifact.EndorsementCount),
			})
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

	type resultJSON struct {
		Layer           string   `json:"layer"`
		ID              string   `json:"id"`
		Score           float64  `json:"score"`
		Content         string   `json:"content,omitempty"`
		Title           string   `json:"title,omitempty"`
		MemoryType      string   `json:"memory_type,omitempty"`
		KnowledgeType   string   `json:"knowledge_type,omitempty"`
		SourceRef       string   `json:"source_ref,omitempty"`
		Visibility      string   `json:"visibility,omitempty"`
		Status          string   `json:"status,omitempty"`
		Endorsements    int      `json:"endorsements,omitempty"`
		Slug            string   `json:"slug,omitempty"`
		Name            string   `json:"name,omitempty"`
		Description     string   `json:"description,omitempty"`
		AgentTypes      []string `json:"agent_types,omitempty"`
		InvocationCount int      `json:"invocation_count,omitempty"`
		Installed       bool     `json:"installed,omitempty"`
	}

	out := make([]resultJSON, 0, len(merged))
	for _, r := range merged {
		out = append(out, resultJSON{
			Layer:           string(r.Layer),
			ID:              r.ID.String(),
			Score:           r.Score,
			Content:         r.Content,
			Title:           r.Title,
			MemoryType:      r.MemoryType,
			KnowledgeType:   r.KnowledgeType,
			SourceRef:       r.SourceRef,
			Visibility:      r.Visibility,
			Status:          r.Status,
			Endorsements:    r.Endorsements,
			Slug:            r.Slug,
			Name:            r.Name,
			Description:     r.Description,
			AgentTypes:      r.AgentTypes,
			InvocationCount: r.InvocationCount,
			Installed:       r.Installed,
		})
	}

	payload, _ := json.Marshal(map[string]any{"results": out})
	return mcpgo.NewToolResultText(string(payload)), nil
}
