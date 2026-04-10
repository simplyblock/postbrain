//go:build integration

package mcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	mcpapi "github.com/simplyblock/postbrain/internal/api/mcp"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestMCP_CrossScopeContext_PermissionByLayer(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "mcp-csc-layer-user-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "mcp-csc-layer-scope-"+uuid.NewString(), nil, principal.ID)
	testhelper.CreateTestEmbeddingModel(t, pool)

	tool := mcpapi.NewServer(pool, svc, cfg).MCPServer().GetTool("cross_scope_context")
	if tool == nil {
		t.Fatal("cross_scope_context tool not registered")
	}

	ctxMemory := withAuthContextPerms(ctx, pool, principal.ID, scope.ID, []string{"memories:read"})
	ctxKnowledge := withAuthContextPerms(ctx, pool, principal.ID, scope.ID, []string{"knowledge:read"})

	t.Run("memory layer allowed with memories:read", func(t *testing.T) {
		req := mcpgo.CallToolRequest{}
		req.Params.Name = "cross_scope_context"
		req.Params.Arguments = map[string]any{
			"query":          "docs consistency",
			"baseline_scope": "project:" + scope.ExternalID,
			"layers":         []any{"memory"},
		}
		result, err := tool.Handler(ctxMemory, req)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if result == nil || result.IsError {
			t.Fatalf("expected success, got %+v", result)
		}
	})

	t.Run("knowledge layer denied with memories:read only", func(t *testing.T) {
		req := mcpgo.CallToolRequest{}
		req.Params.Name = "cross_scope_context"
		req.Params.Arguments = map[string]any{
			"query":          "docs consistency",
			"baseline_scope": "project:" + scope.ExternalID,
			"layers":         []any{"knowledge"},
		}
		result, err := tool.Handler(ctxMemory, req)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if result == nil || !result.IsError {
			t.Fatalf("expected error, got %+v", result)
		}
		msg := firstToolText(result)
		if !strings.Contains(msg, "forbidden: scope access denied") {
			t.Fatalf("error text = %q, want forbidden: scope access denied", msg)
		}
	})

	t.Run("knowledge layer allowed with knowledge:read", func(t *testing.T) {
		req := mcpgo.CallToolRequest{}
		req.Params.Name = "cross_scope_context"
		req.Params.Arguments = map[string]any{
			"query":          "docs consistency",
			"baseline_scope": "project:" + scope.ExternalID,
			"layers":         []any{"knowledge"},
		}
		result, err := tool.Handler(ctxKnowledge, req)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if result == nil || result.IsError {
			t.Fatalf("expected success, got %+v", result)
		}
	})

	t.Run("mixed layers require both permissions", func(t *testing.T) {
		req := mcpgo.CallToolRequest{}
		req.Params.Name = "cross_scope_context"
		req.Params.Arguments = map[string]any{
			"query":          "docs consistency",
			"baseline_scope": "project:" + scope.ExternalID,
			"layers":         []any{"memory", "knowledge"},
		}
		result, err := tool.Handler(ctxMemory, req)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if result == nil || !result.IsError {
			t.Fatalf("expected error, got %+v", result)
		}
		msg := firstToolText(result)
		if !strings.Contains(msg, "forbidden: scope access denied") {
			t.Fatalf("error text = %q, want forbidden: scope access denied", msg)
		}
	})
}

func TestMCP_CrossScopeContext_ComparisonScopeDeniedIsSkipped(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "mcp-csc-skip-user-"+uuid.NewString())
	baseline := testhelper.CreateTestScope(t, pool, "project", "mcp-csc-baseline-"+uuid.NewString(), nil, principal.ID)
	comparisonBlocked := testhelper.CreateTestScope(t, pool, "project", "mcp-csc-blocked-"+uuid.NewString(), nil, principal.ID)
	testhelper.CreateTestEmbeddingModel(t, pool)

	tool := mcpapi.NewServer(pool, svc, cfg).MCPServer().GetTool("cross_scope_context")
	if tool == nil {
		t.Fatal("cross_scope_context tool not registered")
	}

	ctxMemory := withAuthContextPerms(ctx, pool, principal.ID, baseline.ID, []string{"memories:read"})

	req := mcpgo.CallToolRequest{}
	req.Params.Name = "cross_scope_context"
	req.Params.Arguments = map[string]any{
		"query":             "docs consistency",
		"baseline_scope":    "project:" + baseline.ExternalID,
		"comparison_scopes": []any{"project:" + comparisonBlocked.ExternalID},
		"layers":            []any{"memory"},
	}
	result, err := tool.Handler(ctxMemory, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success, got %+v", result)
	}

	var payload struct {
		SkippedScopes []struct {
			Scope  string `json:"scope"`
			Layer  string `json:"layer"`
			Reason string `json:"reason"`
		} `json:"skipped_scopes"`
	}
	if err := json.Unmarshal([]byte(firstToolText(result)), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(payload.SkippedScopes) != 1 {
		t.Fatalf("len(skipped_scopes)=%d, want 1", len(payload.SkippedScopes))
	}
	if payload.SkippedScopes[0].Scope != "project:"+comparisonBlocked.ExternalID {
		t.Fatalf("skipped scope = %q, want %q", payload.SkippedScopes[0].Scope, "project:"+comparisonBlocked.ExternalID)
	}
	if payload.SkippedScopes[0].Layer != "memory" {
		t.Fatalf("skipped layer = %q, want memory", payload.SkippedScopes[0].Layer)
	}
	if payload.SkippedScopes[0].Reason != "forbidden" {
		t.Fatalf("skipped reason = %q, want forbidden", payload.SkippedScopes[0].Reason)
	}
}

func TestMCP_CrossScopeContext_ComparisonScopeDeniedIsSkipped_StableLayerOrder(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "mcp-csc-skip-order-user-"+uuid.NewString())
	baseline := testhelper.CreateTestScope(t, pool, "project", "mcp-csc-skip-order-baseline-"+uuid.NewString(), nil, principal.ID)
	comparisonBlocked := testhelper.CreateTestScope(t, pool, "project", "mcp-csc-skip-order-blocked-"+uuid.NewString(), nil, principal.ID)
	testhelper.CreateTestEmbeddingModel(t, pool)

	tool := mcpapi.NewServer(pool, svc, cfg).MCPServer().GetTool("cross_scope_context")
	if tool == nil {
		t.Fatal("cross_scope_context tool not registered")
	}

	ctxBaselineOnly := withAuthContextPerms(ctx, pool, principal.ID, baseline.ID, []string{"memories:read", "knowledge:read"})

	req := mcpgo.CallToolRequest{}
	req.Params.Name = "cross_scope_context"
	req.Params.Arguments = map[string]any{
		"query":             "docs consistency",
		"baseline_scope":    "project:" + baseline.ExternalID,
		"comparison_scopes": []any{"project:" + comparisonBlocked.ExternalID},
		"layers":            []any{"knowledge", "memory"},
	}
	result, err := tool.Handler(ctxBaselineOnly, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success, got %+v", result)
	}

	var payload struct {
		SkippedScopes []struct {
			Scope  string `json:"scope"`
			Layer  string `json:"layer"`
			Reason string `json:"reason"`
		} `json:"skipped_scopes"`
	}
	if err := json.Unmarshal([]byte(firstToolText(result)), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(payload.SkippedScopes) != 2 {
		t.Fatalf("len(skipped_scopes)=%d, want 2", len(payload.SkippedScopes))
	}
	if payload.SkippedScopes[0].Layer != "memory" || payload.SkippedScopes[1].Layer != "knowledge" {
		t.Fatalf("skipped layer order = [%q, %q], want [memory, knowledge]", payload.SkippedScopes[0].Layer, payload.SkippedScopes[1].Layer)
	}
	for _, skipped := range payload.SkippedScopes {
		if skipped.Scope != "project:"+comparisonBlocked.ExternalID {
			t.Fatalf("skipped scope = %q, want %q", skipped.Scope, "project:"+comparisonBlocked.ExternalID)
		}
		if skipped.Reason != "forbidden" {
			t.Fatalf("skipped reason = %q, want forbidden", skipped.Reason)
		}
	}
}
