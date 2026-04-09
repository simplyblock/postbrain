package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

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

// handleCrossScopeContext validates cross-scope context request arguments.
// Retrieval orchestration is implemented in subsequent phases.
func (s *Server) handleCrossScopeContext(_ context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	query, _ := args["query"].(string)
	if query == "" {
		return mcpgo.NewToolResultError("cross_scope_context: 'query' is required"), nil
	}
	baselineScope, _ := args["baseline_scope"].(string)
	if baselineScope == "" {
		return mcpgo.NewToolResultError("cross_scope_context: 'baseline_scope' is required"), nil
	}
	if _, _, err := parseScopeString(baselineScope); err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("cross_scope_context: invalid baseline_scope: %v", err)), nil
	}

	if _, err := parseCrossScopeLayers(args); err != nil {
		return mcpgo.NewToolResultError(err.Error()), nil
	}
	var comparisonRaw []any
	if v, ok := args["comparison_scopes"].([]any); ok {
		comparisonRaw = v
	}
	if _, err := normalizeAndDeduplicateScopes(comparisonRaw); err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("cross_scope_context: invalid comparison_scopes: %v", err)), nil
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

	limit := 10
	if v, ok := args["limit_per_scope"].(float64); ok {
		limit = int(v)
	}
	if limit <= 0 {
		return mcpgo.NewToolResultError("cross_scope_context: 'limit_per_scope' must be > 0"), nil
	}

	payload, _ := json.Marshal(map[string]any{
		"query":            query,
		"baseline_scope":   baselineScope,
		"baseline_results": []any{},
		"scope_contexts":   []any{},
		"skipped_scopes":   []any{},
	})
	return mcpgo.NewToolResultText(string(payload)), nil
}
