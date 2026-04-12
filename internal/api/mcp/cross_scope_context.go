package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	scopeauthapi "github.com/simplyblock/postbrain/internal/api/scopeauth"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/authz"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/retrieval"
)

var crossScopeLayerOrder = []string{"memory", "knowledge"}

func orderedRequestedCrossScopeLayers(active map[string]bool) []string {
	ordered := make([]string, 0, len(crossScopeLayerOrder))
	for _, layer := range crossScopeLayerOrder {
		if active[layer] {
			ordered = append(ordered, layer)
		}
	}
	return ordered
}

func normalizeAndDeduplicateScopes(raw []any) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		scope, ok := item.(string)
		if !ok || scope == "" {
			return nil, fmt.Errorf("invalid scope value")
		}
		if _, _, err := parseScopeString(scope); err != nil {
			return nil, err
		}
		if _, exists := seen[scope]; exists {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	return out, nil
}

func parseRFC3339Arg(args map[string]any, key string) (*time.Time, error) {
	v, ok := args[key].(string)
	if !ok || v == "" {
		return nil, nil
	}
	ts, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil, fmt.Errorf("cross_scope_context: invalid %s: %v", key, err)
	}
	return &ts, nil
}

func parseCrossScopeLayers(args map[string]any) (map[string]bool, error) {
	active := map[string]bool{
		"memory":    true,
		"knowledge": true,
	}
	raw, ok := args["layers"].([]any)
	if !ok || len(raw) == 0 {
		return active, nil
	}
	active = map[string]bool{}
	for _, item := range raw {
		layer, ok := item.(string)
		if !ok || layer == "" {
			return nil, fmt.Errorf("cross_scope_context: invalid layer")
		}
		switch layer {
		case "memory", "knowledge":
			active[layer] = true
		default:
			return nil, fmt.Errorf("cross_scope_context: invalid layer '%s'", layer)
		}
	}
	return active, nil
}

func resolveScopeID(ctx context.Context, pool *pgxpool.Pool, scopeStr string) (uuid.UUID, error) {
	kind, externalID, err := parseScopeString(scopeStr)
	if err != nil {
		return uuid.Nil, err
	}
	scope, err := db.GetScopeByExternalID(ctx, pool, kind, externalID)
	if err != nil {
		return uuid.Nil, err
	}
	if scope == nil {
		return uuid.Nil, fmt.Errorf("scope '%s' not found", scopeStr)
	}
	return scope.ID, nil
}

func requiredPermissionForLayer(layer string) authz.Permission {
	switch layer {
	case "memory":
		return "memories:read"
	case "knowledge":
		return "knowledge:read"
	default:
		return ""
	}
}

func activeRetrievalLayersForScope(allLayers map[string]bool, denied map[string]bool) map[retrieval.Layer]bool {
	active := map[retrieval.Layer]bool{
		retrieval.LayerMemory:    false,
		retrieval.LayerKnowledge: false,
		retrieval.LayerSkill:     false,
	}
	if allLayers["memory"] && !denied["memory"] {
		active[retrieval.LayerMemory] = true
	}
	if allLayers["knowledge"] && !denied["knowledge"] {
		active[retrieval.LayerKnowledge] = true
	}
	return active
}

func asCrossScopeResultJSON(scope string, results []*retrieval.Result) []map[string]any {
	out := make([]map[string]any, 0, len(results))
	for _, r := range results {
		score := r.Score
		if math.IsNaN(score) || math.IsInf(score, 0) {
			score = 0
		}
		var sourceRef any
		if r.SourceRef != "" {
			sourceRef = r.SourceRef
		} else {
			sourceRef = nil
		}
		item := map[string]any{
			"scope":                  scope,
			"layer":                  string(r.Layer),
			"id":                     r.ID.String(),
			"score":                  score,
			"content":                r.Content,
			"title":                  r.Title,
			"memory_type":            r.MemoryType,
			"knowledge_type":         r.KnowledgeType,
			"artifact_kind":          r.ArtifactKind,
			"source_ref":             sourceRef,
			"visibility":             r.Visibility,
			"status":                 r.Status,
			"endorsements":           r.Endorsements,
			"full_content_available": r.FullContentAvailable,
			"slug":                   r.Slug,
			"name":                   r.Name,
			"description":            r.Description,
			"agent_types":            r.AgentTypes,
			"invocation_count":       r.InvocationCount,
			"installed":              r.Installed,
			"graph_context":          r.GraphContext,
		}
		if !r.CreatedAt.IsZero() {
			item["created_at"] = r.CreatedAt.UTC().Format(time.RFC3339)
		} else {
			item["created_at"] = nil
		}
		if !r.UpdatedAt.IsZero() {
			item["updated_at"] = r.UpdatedAt.UTC().Format(time.RFC3339)
		} else {
			item["updated_at"] = nil
		}
		out = append(out, item)
	}
	return out
}

// handleCrossScopeContext validates cross-scope context request arguments.
// Retrieval orchestration is implemented in subsequent phases.
func (s *Server) handleCrossScopeContext(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	query := argString(args, "query")
	if query == "" {
		return mcpgo.NewToolResultError("cross_scope_context: 'query' is required"), nil
	}
	baselineScope := argString(args, "baseline_scope")
	if baselineScope == "" {
		return mcpgo.NewToolResultError("cross_scope_context: 'baseline_scope' is required"), nil
	}
	if _, _, err := parseScopeString(baselineScope); err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("cross_scope_context: invalid baseline_scope: %v", err)), nil
	}

	layers, err := parseCrossScopeLayers(args)
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	var comparisonRaw []any
	if v, ok := args["comparison_scopes"].([]any); ok {
		comparisonRaw = v
	}

	since, err := parseRFC3339Arg(args, "since")
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	until, err := parseRFC3339Arg(args, "until")
	if err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	if since != nil && until != nil && since.After(*until) {
		return mcpgo.NewToolResultError("cross_scope_context: invalid time window: since must be <= until"), nil
	}

	limit := argIntOrDefault(args, "limit_per_scope", 10)
	if limit <= 0 {
		return mcpgo.NewToolResultError("cross_scope_context: 'limit_per_scope' must be > 0"), nil
	}
	minScore := argFloat64OrDefault(args, "min_score", 0.0)
	searchMode := argString(args, "search_mode")
	if searchMode == "" {
		searchMode = "hybrid"
	}
	graphDepth := parseGraphDepthWithDefault(args, defaultCrossScopeGraphDepth)

	if s.pool == nil {
		return mcpgo.NewToolResultError("cross_scope_context: server not configured (no database connection)"), nil
	}
	principalID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	authorizedScopeIDs, err := s.effectiveScopeIDsForRequest(ctx)
	if err != nil {
		return mcpgo.NewToolResultError("cross_scope_context: scope authorization failed"), nil
	}

	baselineScopeID, err := resolveScopeID(ctx, s.pool, baselineScope)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("cross_scope_context: baseline scope lookup: %v", err)), nil
	}
	for _, layer := range orderedRequestedCrossScopeLayers(layers) {
		perm := requiredPermissionForLayer(layer)
		if perm == "" {
			continue
		}
		if err := scopeauthapi.AuthorizeContextScope(ctx, baselineScopeID, perm); err != nil {
			return scopeAuthzToolError(ctx, "cross_scope_context", baselineScopeID, err), nil
		}
	}

	baselineResults, err := retrieval.OrchestrateCrossScopeContext(ctx, retrieval.OrchestrateDeps{
		Pool:     s.pool,
		MemStore: s.memStore,
		KnwStore: s.knwStore,
		SklStore: s.sklStore,
		Svc:      s.svc,
	}, retrieval.OrchestrateInput{
		Query:              query,
		ScopeID:            baselineScopeID,
		PrincipalID:        principalID,
		AuthorizedScopeIDs: authorizedScopeIDs,
		SearchMode:         searchMode,
		Limit:              limit,
		MinScore:           minScore,
		GraphDepth:         graphDepth,
		ActiveLayers:       activeRetrievalLayersForScope(layers, map[string]bool{}),
		Since:              since,
		Until:              until,
	})
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("cross_scope_context: baseline retrieval failed: %v", err)), nil
	}

	comparisonScopes, err := normalizeAndDeduplicateScopes(comparisonRaw)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("cross_scope_context: invalid comparison_scopes: %v", err)), nil
	}
	skippedScopes := make([]map[string]any, 0)
	scopeContexts := make([]map[string]any, 0, len(comparisonScopes))
	for _, scopeStr := range comparisonScopes {
		scopeID, err := resolveScopeID(ctx, s.pool, scopeStr)
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("cross_scope_context: comparison scope lookup: %v", err)), nil
		}
		deniedLayers := map[string]bool{}
		for _, layer := range orderedRequestedCrossScopeLayers(layers) {
			perm := requiredPermissionForLayer(layer)
			if perm == "" {
				continue
			}
			if err := scopeauthapi.AuthorizeContextScope(ctx, scopeID, perm); err != nil {
				deniedLayers[layer] = true
				skippedScopes = append(skippedScopes, map[string]any{
					"scope":  scopeStr,
					"layer":  layer,
					"reason": "forbidden",
				})
			}
		}

		activeLayers := activeRetrievalLayersForScope(layers, deniedLayers)
		if !activeLayers[retrieval.LayerMemory] && !activeLayers[retrieval.LayerKnowledge] {
			continue
		}
		results, err := retrieval.OrchestrateCrossScopeContext(ctx, retrieval.OrchestrateDeps{
			Pool:     s.pool,
			MemStore: s.memStore,
			KnwStore: s.knwStore,
			SklStore: s.sklStore,
			Svc:      s.svc,
		}, retrieval.OrchestrateInput{
			Query:              query,
			ScopeID:            scopeID,
			PrincipalID:        principalID,
			AuthorizedScopeIDs: authorizedScopeIDs,
			SearchMode:         searchMode,
			Limit:              limit,
			MinScore:           minScore,
			GraphDepth:         graphDepth,
			ActiveLayers:       activeLayers,
			Since:              since,
			Until:              until,
		})
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("cross_scope_context: comparison retrieval failed: %v", err)), nil
		}
		scopeContexts = append(scopeContexts, map[string]any{
			"scope":   scopeStr,
			"results": asCrossScopeResultJSON(scopeStr, results),
		})
	}

	var sinceOut any
	if since != nil {
		sinceOut = since.UTC().Format(time.RFC3339)
	}
	var untilOut any
	if until != nil {
		untilOut = until.UTC().Format(time.RFC3339)
	}

	payload, err := json.Marshal(map[string]any{
		"query":            query,
		"time_window":      map[string]any{"since": sinceOut, "until": untilOut},
		"baseline_scope":   baselineScope,
		"baseline_results": asCrossScopeResultJSON(baselineScope, baselineResults),
		"scope_contexts":   scopeContexts,
		"skipped_scopes":   skippedScopes,
	})
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("cross_scope_context: marshal response: %v", err)), nil
	}
	return mcpgo.NewToolResultText(string(payload)), nil
}
