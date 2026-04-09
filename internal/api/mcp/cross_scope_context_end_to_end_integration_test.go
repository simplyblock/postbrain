//go:build integration

package mcp_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	mcpapi "github.com/simplyblock/postbrain/internal/api/mcp"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestMCP_CrossScopeContext_EndToEnd_MixedLayers_TimeWindow_AuthzAndProvenance(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}
	testhelper.CreateTestEmbeddingModel(t, pool)

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "mcp-csc-e2e-user-"+uuid.NewString())
	blockedOwner := testhelper.CreateTestPrincipal(t, pool, "user", "mcp-csc-e2e-blocked-owner-"+uuid.NewString())

	baselineScope := testhelper.CreateTestScope(t, pool, "project", "mcp-csc-e2e-docs-"+uuid.NewString(), nil, principal.ID)
	sourceScope := testhelper.CreateTestScope(t, pool, "project", "mcp-csc-e2e-source-"+uuid.NewString(), nil, principal.ID)
	blockedScope := testhelper.CreateTestScope(t, pool, "project", "mcp-csc-e2e-blocked-"+uuid.NewString(), nil, blockedOwner.ID)

	srv := mcpapi.NewServer(pool, svc, cfg).MCPServer()
	tool := srv.GetTool("cross_scope_context")
	if tool == nil {
		t.Fatal("cross_scope_context tool not registered")
	}

	ctxBaselineWrite := withAuthContextPerms(ctx, pool, principal.ID, baselineScope.ID, []string{"memories:write", "memories:read"})
	ctxSourceWrite := withAuthContextPerms(ctx, pool, principal.ID, sourceScope.ID, []string{"memories:write", "memories:read"})

	baselineMemoryOld := createMemoryViaRemember(t, srv, ctxBaselineWrite, "project:"+baselineScope.ExternalID, "phase7 e2e verification token baseline memory old")
	baselineMemoryNew := createMemoryViaRemember(t, srv, ctxBaselineWrite, "project:"+baselineScope.ExternalID, "phase7 e2e verification token baseline memory new")
	sourceMemoryOld := createMemoryViaRemember(t, srv, ctxSourceWrite, "project:"+sourceScope.ExternalID, "phase7 e2e verification token source memory old")
	sourceMemoryNew := createMemoryViaRemember(t, srv, ctxSourceWrite, "project:"+sourceScope.ExternalID, "phase7 e2e verification token source memory new")

	if _, err := pool.Exec(ctx, `UPDATE memories SET created_at = now() - interval '72 hours' WHERE id = ANY($1::uuid[])`, []uuid.UUID{baselineMemoryOld, sourceMemoryOld}); err != nil {
		t.Fatalf("set old memory created_at timestamps: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE memories SET created_at = now() - interval '2 hours' WHERE id = ANY($1::uuid[])`, []uuid.UUID{baselineMemoryNew, sourceMemoryNew}); err != nil {
		t.Fatalf("set new memory created_at timestamps: %v", err)
	}

	baselineArtifactOld := testhelper.CreateTestArtifact(t, pool, baselineScope.ID, principal.ID, "phase7 e2e verification token baseline artifact old").ID
	baselineArtifactNew := testhelper.CreateTestArtifact(t, pool, baselineScope.ID, principal.ID, "phase7 e2e verification token baseline artifact new").ID
	sourceArtifactOld := testhelper.CreateTestArtifact(t, pool, sourceScope.ID, principal.ID, "phase7 e2e verification token source artifact old").ID
	sourceArtifactNew := testhelper.CreateTestArtifact(t, pool, sourceScope.ID, principal.ID, "phase7 e2e verification token source artifact new").ID

	if _, err := pool.Exec(ctx, `UPDATE knowledge_artifacts SET published_at = now() - interval '72 hours', created_at = now() - interval '2 hours' WHERE id = ANY($1::uuid[])`, []uuid.UUID{baselineArtifactOld, sourceArtifactOld}); err != nil {
		t.Fatalf("set old artifact timestamps: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE knowledge_artifacts SET published_at = now() - interval '2 hours', created_at = now() - interval '72 hours' WHERE id = ANY($1::uuid[])`, []uuid.UUID{baselineArtifactNew, sourceArtifactNew}); err != nil {
		t.Fatalf("set new artifact timestamps: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE knowledge_artifacts SET embedding='[1,0,0,0]'::vector WHERE id = ANY($1::uuid[])`, []uuid.UUID{baselineArtifactOld, baselineArtifactNew, sourceArtifactOld, sourceArtifactNew}); err != nil {
		t.Fatalf("set artifact embeddings: %v", err)
	}

	ctxRead := withAuthContextUnrestricted(ctx, pool, principal.ID)

	since := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	until := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)

	req := mcpgo.CallToolRequest{}
	req.Params.Name = "cross_scope_context"
	req.Params.Arguments = map[string]any{
		"query":             "phase7 e2e verification token",
		"baseline_scope":    "project:" + baselineScope.ExternalID,
		"comparison_scopes": []any{"project:" + sourceScope.ExternalID, "project:" + blockedScope.ExternalID},
		"layers":            []any{"memory", "knowledge"},
		"since":             since,
		"until":             until,
		"limit_per_scope":   float64(20),
	}
	result, err := tool.Handler(ctxRead, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected success, got %+v", result)
	}

	var payload struct {
		Query         string `json:"query"`
		BaselineScope string `json:"baseline_scope"`
		TimeWindow    struct {
			Since string `json:"since"`
			Until string `json:"until"`
		} `json:"time_window"`
		BaselineResults []map[string]any `json:"baseline_results"`
		ScopeContexts   []struct {
			Scope   string           `json:"scope"`
			Results []map[string]any `json:"results"`
		} `json:"scope_contexts"`
		SkippedScopes []struct {
			Scope  string `json:"scope"`
			Layer  string `json:"layer"`
			Reason string `json:"reason"`
		} `json:"skipped_scopes"`
	}
	if err := json.Unmarshal([]byte(firstToolText(result)), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if payload.Query != "phase7 e2e verification token" {
		t.Fatalf("query=%q, want phase7 e2e verification token", payload.Query)
	}
	if payload.BaselineScope != "project:"+baselineScope.ExternalID {
		t.Fatalf("baseline_scope=%q, want %q", payload.BaselineScope, "project:"+baselineScope.ExternalID)
	}
	if payload.TimeWindow.Since != since || payload.TimeWindow.Until != until {
		t.Fatalf("time_window=%+v, want since=%q until=%q", payload.TimeWindow, since, until)
	}

	if containsID(payload.BaselineResults, baselineMemoryOld.String()) {
		t.Fatalf("unexpected old baseline memory %s in baseline_results", baselineMemoryOld)
	}
	if containsID(payload.BaselineResults, baselineArtifactOld.String()) {
		t.Fatalf("unexpected old baseline artifact %s in baseline_results", baselineArtifactOld)
	}
	if !containsID(payload.BaselineResults, baselineMemoryNew.String()) {
		t.Fatalf("expected new baseline memory %s in baseline_results", baselineMemoryNew)
	}
	if !containsID(payload.BaselineResults, baselineArtifactNew.String()) {
		t.Fatalf("expected new baseline artifact %s in baseline_results", baselineArtifactNew)
	}
	if !resultsContainOnlyScope(payload.BaselineResults, "project:"+baselineScope.ExternalID) {
		t.Fatal("baseline_results contains unexpected scope provenance")
	}

	if len(payload.ScopeContexts) != 1 {
		t.Fatalf("len(scope_contexts)=%d, want 1", len(payload.ScopeContexts))
	}
	if payload.ScopeContexts[0].Scope != "project:"+sourceScope.ExternalID {
		t.Fatalf("scope_contexts[0].scope=%q, want %q", payload.ScopeContexts[0].Scope, "project:"+sourceScope.ExternalID)
	}
	if containsID(payload.ScopeContexts[0].Results, sourceMemoryOld.String()) {
		t.Fatalf("unexpected old source memory %s in source results", sourceMemoryOld)
	}
	if containsID(payload.ScopeContexts[0].Results, sourceArtifactOld.String()) {
		t.Fatalf("unexpected old source artifact %s in source results", sourceArtifactOld)
	}
	if !containsID(payload.ScopeContexts[0].Results, sourceMemoryNew.String()) {
		t.Fatalf("expected new source memory %s in source results", sourceMemoryNew)
	}
	if !containsID(payload.ScopeContexts[0].Results, sourceArtifactNew.String()) {
		t.Fatalf("expected new source artifact %s in source results", sourceArtifactNew)
	}
	if !resultsContainOnlyScope(payload.ScopeContexts[0].Results, "project:"+sourceScope.ExternalID) {
		t.Fatal("source comparison results contains unexpected scope provenance")
	}

	if len(payload.SkippedScopes) != 2 {
		t.Fatalf("len(skipped_scopes)=%d, want 2", len(payload.SkippedScopes))
	}
	if !hasSkippedScopeLayer(payload.SkippedScopes, "project:"+blockedScope.ExternalID, "memory") {
		t.Fatalf("expected skipped_scopes to include blocked memory layer for %s", blockedScope.ExternalID)
	}
	if !hasSkippedScopeLayer(payload.SkippedScopes, "project:"+blockedScope.ExternalID, "knowledge") {
		t.Fatalf("expected skipped_scopes to include blocked knowledge layer for %s", blockedScope.ExternalID)
	}
	for _, skipped := range payload.SkippedScopes {
		if skipped.Reason != "forbidden" {
			t.Fatalf("skipped reason=%q, want forbidden", skipped.Reason)
		}
	}

	assertProvenanceKeysPresent(t, payload.BaselineResults)
	assertProvenanceKeysPresent(t, payload.ScopeContexts[0].Results)
}

func containsID(results []map[string]any, wantID string) bool {
	for _, result := range results {
		if gotID, _ := result["id"].(string); gotID == wantID {
			return true
		}
	}
	return false
}

func resultsContainOnlyScope(results []map[string]any, wantScope string) bool {
	for _, result := range results {
		scope, _ := result["scope"].(string)
		if scope != wantScope {
			return false
		}
	}
	return true
}

func hasSkippedScopeLayer(skipped []struct {
	Scope  string `json:"scope"`
	Layer  string `json:"layer"`
	Reason string `json:"reason"`
}, scope, layer string) bool {
	for _, entry := range skipped {
		if entry.Scope == scope && entry.Layer == layer {
			return true
		}
	}
	return false
}

func assertProvenanceKeysPresent(t *testing.T, results []map[string]any) {
	t.Helper()
	keys := []string{"scope", "layer", "id", "score", "source_ref", "created_at", "updated_at"}
	for i, result := range results {
		for _, key := range keys {
			if _, ok := result[key]; !ok {
				t.Fatalf("results[%d] missing provenance key %q", i, key)
			}
		}
	}
}
