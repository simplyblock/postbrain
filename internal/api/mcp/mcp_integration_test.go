//go:build integration

package mcp_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	mcpapi "github.com/simplyblock/postbrain/internal/api/mcp"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestMCP_Remember_Recall_Forget(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "mcp-e2e-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "mcp-e2e-project", nil, principal.ID)
	testhelper.CreateTestEmbeddingModel(t, pool)

	srv := mcpapi.NewServer(pool, svc, cfg)
	mcpSrv := srv.MCPServer()

	scopeStr := "project:" + scope.ExternalID
	ctx = withAuthContext(ctx, principal.ID, scope.ID)

	// Test remember.
	rememberTool := mcpSrv.GetTool("remember")
	if rememberTool == nil {
		t.Fatal("remember tool not registered")
	}

	req := mcpgo.CallToolRequest{}
	req.Params.Name = "remember"
	firstSummary := "Long-form note about goroutine communication."
	req.Params.Arguments = map[string]any{
		"content":     "E2E test: Go channels are goroutine-safe communication primitives",
		"summary":     firstSummary,
		"scope":       scopeStr,
		"memory_type": "semantic",
	}

	result, err := rememberTool.Handler(ctx, req)
	if err != nil {
		t.Fatalf("remember failed: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("remember returned error result: %+v", result)
	}

	// Verify content is non-empty.
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty result content")
	}
	text, ok := result.Content[0].(mcpgo.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	var rememberOut map[string]any
	if err := json.Unmarshal([]byte(text.Text), &rememberOut); err != nil {
		t.Fatalf("remember output is not JSON: %v", err)
	}
	memIDStr, _ := rememberOut["memory_id"].(string)
	memID, err := uuid.Parse(memIDStr)
	if err != nil {
		t.Fatalf("invalid memory_id %q: %v", memIDStr, err)
	}
	mem, err := db.GetMemory(ctx, pool, memID)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if mem == nil {
		t.Fatal("expected memory to exist after remember")
	}
	if mem.Summary == nil || *mem.Summary != firstSummary {
		t.Fatalf("summary = %v, want %q", mem.Summary, firstSummary)
	}
	var meta map[string]any
	if err := json.Unmarshal(mem.Meta, &meta); err != nil {
		t.Fatalf("memory meta is not valid JSON: %v", err)
	}
	if got, _ := meta["content_style"].(string); got != "long" {
		t.Fatalf("meta.content_style = %q, want %q", got, "long")
	}

	// Remember same content again to hit near-duplicate update path and ensure
	// summary can be updated there too.
	secondSummary := "Updated long-form guidance for goroutine communication."
	req2 := mcpgo.CallToolRequest{}
	req2.Params.Name = "remember"
	req2.Params.Arguments = map[string]any{
		"content":     "E2E test: Go channels are goroutine-safe communication primitives",
		"summary":     secondSummary,
		"scope":       scopeStr,
		"memory_type": "semantic",
	}
	result2, err := rememberTool.Handler(ctx, req2)
	if err != nil {
		t.Fatalf("remember duplicate failed: %v", err)
	}
	if result2 == nil || result2.IsError {
		t.Fatalf("remember duplicate returned error result: %+v", result2)
	}
	memAfter, err := db.GetMemory(ctx, pool, memID)
	if err != nil {
		t.Fatalf("GetMemory after duplicate remember: %v", err)
	}
	if memAfter == nil {
		t.Fatal("expected memory after duplicate remember")
	}
	if memAfter.Summary == nil || *memAfter.Summary != secondSummary {
		t.Fatalf("summary after duplicate remember = %v, want %q", memAfter.Summary, secondSummary)
	}

	// Test recall.
	recallTool := mcpSrv.GetTool("recall")
	if recallTool == nil {
		t.Fatal("recall tool not registered")
	}

	recallReq := mcpgo.CallToolRequest{}
	recallReq.Params.Name = "recall"
	recallReq.Params.Arguments = map[string]any{
		"query": "goroutine communication",
		"scope": scopeStr,
	}

	recallResult, err := recallTool.Handler(ctx, recallReq)
	if err != nil {
		t.Fatalf("recall failed: %v", err)
	}
	if recallResult == nil {
		t.Fatal("recall returned nil result")
	}
}

func TestMCP_Publish_Endorse_AutoPublish(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "mcp-publish-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "mcp-publish-project", nil, principal.ID)
	testhelper.CreateTestEmbeddingModel(t, pool)

	srv := mcpapi.NewServer(pool, svc, cfg)
	mcpSrv := srv.MCPServer()

	scopeStr := "project:" + scope.ExternalID
	ctx = withAuthContext(ctx, principal.ID, scope.ID)

	// Publish an artifact.
	publishTool := mcpSrv.GetTool("publish")
	if publishTool == nil {
		t.Fatal("publish tool not registered")
	}

	pubReq := mcpgo.CallToolRequest{}
	pubReq.Params.Name = "publish"
	pubReq.Params.Arguments = map[string]any{
		"title":          "E2E Test Article",
		"content":        "This is a test knowledge artifact for E2E testing",
		"knowledge_type": "semantic",
		"scope":          scopeStr,
		"auto_review":    true,
	}

	pubResult, err := publishTool.Handler(ctx, pubReq)
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if pubResult == nil || pubResult.IsError {
		t.Fatalf("publish returned error: %+v", pubResult)
	}
}

func withAuthContext(ctx context.Context, principalID, scopeID uuid.UUID) context.Context {
	ctx = context.WithValue(ctx, auth.ContextKeyPrincipalID, principalID)
	token := &db.Token{
		PrincipalID: principalID,
		ScopeIds:    []uuid.UUID{scopeID},
		Permissions: []string{"read", "write"},
	}
	return context.WithValue(ctx, auth.ContextKeyToken, token)
}
