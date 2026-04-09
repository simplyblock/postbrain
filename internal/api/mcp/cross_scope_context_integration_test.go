//go:build integration

package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

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
	if err := json.Unmarshal([]byte(crossScopeResultText(t, result)), &payload); err != nil {
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

func TestMCP_CrossScopeContext_TimeWindowSince_FiltersOldMemory(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "mcp-csc-time-memory-user-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "mcp-csc-time-memory-scope-"+uuid.NewString(), nil, principal.ID)
	testhelper.CreateTestEmbeddingModel(t, pool)

	srv := mcpapi.NewServer(pool, svc, cfg).MCPServer()
	tool := srv.GetTool("cross_scope_context")
	if tool == nil {
		t.Fatal("cross_scope_context tool not registered")
	}

	ctxWrite := withAuthContextPerms(ctx, pool, principal.ID, scope.ID, []string{"memories:write", "memories:read"})
	oldMemID := createMemoryViaRemember(t, srv, ctxWrite, "project:"+scope.ExternalID, "time window memory token qwerty old")
	newMemID := createMemoryViaRemember(t, srv, ctxWrite, "project:"+scope.ExternalID, "time window memory token qwerty new")

	if _, err := pool.Exec(ctx, `UPDATE memories SET created_at = now() - interval '72 hours' WHERE id = $1`, oldMemID); err != nil {
		t.Fatalf("set old memory created_at: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE memories SET created_at = now() - interval '2 hours' WHERE id = $1`, newMemID); err != nil {
		t.Fatalf("set new memory created_at: %v", err)
	}

	ctxRead := withAuthContextPerms(ctx, pool, principal.ID, scope.ID, []string{"memories:read"})

	req := mcpgo.CallToolRequest{}
	req.Params.Name = "cross_scope_context"
	req.Params.Arguments = map[string]any{
		"query":           "time window memory token qwerty",
		"baseline_scope":  "project:" + scope.ExternalID,
		"layers":          []any{"memory"},
		"limit_per_scope": float64(20),
		"since":           time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339),
	}
	result, err := tool.Handler(ctxRead, req)
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
	if err := json.Unmarshal([]byte(crossScopeResultText(t, result)), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	for _, r := range payload.BaselineResults {
		if r.ID == oldMemID.String() {
			t.Fatalf("unexpected old memory %s in since-filtered results", oldMemID)
		}
	}
	foundNew := false
	for _, r := range payload.BaselineResults {
		if r.ID == newMemID.String() {
			foundNew = true
			break
		}
	}
	if !foundNew {
		t.Fatalf("expected new memory %s in since-filtered results", newMemID)
	}
}

func TestMCP_CrossScopeContext_TimeWindowSince_UsesKnowledgePublishedAt(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "mcp-csc-time-knowledge-user-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "mcp-csc-time-knowledge-scope-"+uuid.NewString(), nil, principal.ID)
	testhelper.CreateTestEmbeddingModel(t, pool)

	srv := mcpapi.NewServer(pool, svc, cfg).MCPServer()
	tool := srv.GetTool("cross_scope_context")
	if tool == nil {
		t.Fatal("cross_scope_context tool not registered")
	}

	oldArtifact := testhelper.CreateTestArtifact(t, pool, scope.ID, principal.ID, "time window artifact token qwerty old")
	newArtifact := testhelper.CreateTestArtifact(t, pool, scope.ID, principal.ID, "time window artifact token qwerty new")
	oldArtifactID := oldArtifact.ID
	newArtifactID := newArtifact.ID

	if _, err := pool.Exec(ctx, `UPDATE knowledge_artifacts SET published_at = now() - interval '72 hours', created_at = now() - interval '2 hours' WHERE id = $1`, oldArtifactID); err != nil {
		t.Fatalf("set old artifact timestamps: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE knowledge_artifacts SET published_at = now() - interval '2 hours', created_at = now() - interval '72 hours' WHERE id = $1`, newArtifactID); err != nil {
		t.Fatalf("set new artifact timestamps: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE knowledge_artifacts SET embedding='[1,0,0,0]'::vector WHERE id = ANY($1::uuid[])`, []uuid.UUID{oldArtifactID, newArtifactID}); err != nil {
		t.Fatalf("set artifact embeddings: %v", err)
	}

	ctxRead := withAuthContextPerms(ctx, pool, principal.ID, scope.ID, []string{"knowledge:read"})

	req := mcpgo.CallToolRequest{}
	req.Params.Name = "cross_scope_context"
	req.Params.Arguments = map[string]any{
		"query":           "time window artifact token qwerty",
		"baseline_scope":  "project:" + scope.ExternalID,
		"layers":          []any{"knowledge"},
		"limit_per_scope": float64(20),
		"since":           time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339),
	}
	result, err := tool.Handler(ctxRead, req)
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
	if err := json.Unmarshal([]byte(crossScopeResultText(t, result)), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	for _, r := range payload.BaselineResults {
		if r.ID == oldArtifactID.String() {
			t.Fatalf("unexpected old artifact %s in since-filtered results", oldArtifactID)
		}
	}
	foundNew := false
	for _, r := range payload.BaselineResults {
		if r.ID == newArtifactID.String() {
			foundNew = true
			break
		}
	}
	if !foundNew {
		t.Fatalf("expected new artifact %s in since-filtered results", newArtifactID)
	}
}

func crossScopeResultText(t *testing.T, result *mcpgo.CallToolResult) string {
	t.Helper()
	if result == nil {
		t.Fatal("nil tool result")
	}
	if len(result.Content) == 0 {
		t.Fatal("tool result has empty content")
	}
	switch v := result.Content[0].(type) {
	case mcpgo.TextContent:
		return v.Text
	case *mcpgo.TextContent:
		return v.Text
	default:
		t.Fatalf("unexpected tool content type %T: %v", v, fmt.Sprintf("%v", v))
	}
	return ""
}
