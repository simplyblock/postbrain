//go:build integration

package mcp_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	mcpapi "github.com/simplyblock/postbrain/internal/api/mcp"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestMCP_CrossScopeContext_StrictMemoryScope_ExcludesAncestorMemories(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "mcp-csc-strict-user-"+uuid.NewString())
	ancestor := testhelper.CreateTestScope(t, pool, "project", "mcp-csc-ancestor-"+uuid.NewString(), nil, principal.ID)
	child := testhelper.CreateTestScope(t, pool, "project", "mcp-csc-child-"+uuid.NewString(), &ancestor.ID, principal.ID)
	testhelper.CreateTestEmbeddingModel(t, pool)

	srv := mcpapi.NewServer(pool, svc, cfg).MCPServer()
	tool := srv.GetTool("cross_scope_context")
	if tool == nil {
		t.Fatal("cross_scope_context tool not registered")
	}

	ctxAncestorWrite := withAuthContextPerms(ctx, pool, principal.ID, ancestor.ID, []string{"memories:write", "memories:read"})
	ctxChildWrite := withAuthContextPerms(ctx, pool, principal.ID, child.ID, []string{"memories:write", "memories:read"})
	ancestorMemID := createMemoryViaRemember(t, srv, ctxAncestorWrite, "project:"+ancestor.ExternalID, "strict scope evidence token xyz")
	childMemID := createMemoryViaRemember(t, srv, ctxChildWrite, "project:"+child.ExternalID, "strict scope evidence token xyz")

	ctxAuth := withAuthContextPerms(ctx, pool, principal.ID, child.ID, []string{"memories:read"})

	req := mcpgo.CallToolRequest{}
	req.Params.Name = "cross_scope_context"
	req.Params.Arguments = map[string]any{
		"query":           "strict scope evidence token xyz",
		"baseline_scope":  "project:" + child.ExternalID,
		"layers":          []any{"memory"},
		"limit_per_scope": float64(20),
	}
	result, err := tool.Handler(ctxAuth, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success, got %+v", result)
	}

	var payload struct {
		BaselineResults []struct {
			ID string `json:"id"`
		} `json:"baseline_results"`
	}
	if err := json.Unmarshal([]byte(firstToolText(result)), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(payload.BaselineResults) == 0 {
		t.Fatal("expected baseline_results to include child memory")
	}
	for _, r := range payload.BaselineResults {
		if r.ID == ancestorMemID.String() {
			t.Fatalf("unexpected ancestor memory %s in strict-scope results", ancestorMemID)
		}
	}
	foundChild := false
	for _, r := range payload.BaselineResults {
		if r.ID == childMemID.String() {
			foundChild = true
			break
		}
	}
	if !foundChild {
		t.Fatalf("expected child memory %s in baseline results", childMemID)
	}
}
