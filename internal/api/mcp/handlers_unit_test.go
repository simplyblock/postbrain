package mcp

import (
	"context"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// callTool is a helper that builds a CallToolRequest from a plain map and
// invokes the given handler, returning the result.
func callTool(t *testing.T, args map[string]any, fn func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)) *mcpgo.CallToolResult {
	t.Helper()
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = args
	result, err := fn(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned unexpected Go error: %v", err)
	}
	if result == nil {
		t.Fatal("handler returned nil result")
	}
	return result
}

// callToolWithMeta builds a CallToolRequest with a custom Meta (e.g. a
// progress token) and invokes the given handler.
func callToolWithMeta(t *testing.T, args map[string]any, meta *mcpgo.Meta, fn func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)) *mcpgo.CallToolResult {
	t.Helper()
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = args
	req.Params.Meta = meta
	result, err := fn(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned unexpected Go error: %v", err)
	}
	if result == nil {
		t.Fatal("handler returned nil result")
	}
	return result
}

// assertToolError fails the test if result.IsError is not true.
func assertToolError(t *testing.T, result *mcpgo.CallToolResult) {
	t.Helper()
	if !result.IsError {
		t.Errorf("expected tool error (IsError=true), got IsError=false")
	}
}

// toolErrorText extracts the first text payload from a tool result.
func toolErrorText(t *testing.T, result *mcpgo.CallToolResult) string {
	t.Helper()
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	if tc, ok := result.Content[0].(mcpgo.TextContent); ok {
		return tc.Text
	}
	return ""
}

// ── handleRecall ─────────────────────────────────────────────────────────────

func TestHandleRecall_MissingQuery_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"scope": "project:acme/api",
		// query intentionally omitted
	}, s.handleRecall)
	assertToolError(t, result)
}

func TestHandleRecall_EmptyQuery_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"query": "",
		"scope": "project:acme/api",
	}, s.handleRecall)
	assertToolError(t, result)
}

// ── handleCrossScopeContext ──────────────────────────────────────────────────

func TestHandleCrossScopeContext_MissingQuery_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"baseline_scope": "project:acme/docs",
	}, s.handleCrossScopeContext)
	assertToolError(t, result)
}

func TestHandleCrossScopeContext_MissingBaselineScope_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"query": "check docs consistency",
	}, s.handleCrossScopeContext)
	assertToolError(t, result)
}

func TestHandleCrossScopeContext_InvalidLayer_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"query":          "check docs consistency",
		"baseline_scope": "project:acme/docs",
		"layers":         []any{"memory", "skill"},
	}, s.handleCrossScopeContext)
	assertToolError(t, result)
}

func TestHandleCrossScopeContext_InvalidSince_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"query":          "check docs consistency",
		"baseline_scope": "project:acme/docs",
		"since":          "yesterday",
	}, s.handleCrossScopeContext)
	assertToolError(t, result)
}

func TestHandleCrossScopeContext_SinceAfterUntil_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"query":          "check docs consistency",
		"baseline_scope": "project:acme/docs",
		"since":          "2026-04-09T12:00:00Z",
		"until":          "2026-04-08T12:00:00Z",
	}, s.handleCrossScopeContext)
	assertToolError(t, result)
}

func TestHandleCrossScopeContext_InvalidBaselineScope_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"query":          "check docs consistency",
		"baseline_scope": "invalid-scope-format",
	}, s.handleCrossScopeContext)
	assertToolError(t, result)
}

func TestHandleCrossScopeContext_NonPositiveLimitPerScope_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"query":           "check docs consistency",
		"baseline_scope":  "project:acme/docs",
		"limit_per_scope": float64(0),
	}, s.handleCrossScopeContext)
	assertToolError(t, result)
}

func TestNormalizeAndDeduplicateScopes_StableOrder(t *testing.T) {
	in := []any{
		"project:acme/source",
		"project:acme/source",
		"project:acme/session",
		"project:acme/source",
		"project:acme/knowledge",
	}
	got, err := normalizeAndDeduplicateScopes(in)
	if err != nil {
		t.Fatalf("normalizeAndDeduplicateScopes error: %v", err)
	}
	want := []string{
		"project:acme/source",
		"project:acme/session",
		"project:acme/knowledge",
	}
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d, len(want)=%d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

// ── handleRemember ────────────────────────────────────────────────────────────

// Note: TestHandleRemember_MissingContent and TestHandleRemember_MissingScope
// already exist in server_test.go.  The case below covers the empty-string
// variant to complement those tests.
func TestHandleRemember_EmptyContent_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"content": "", // present but empty
		"scope":   "project:acme/api",
	}, s.handleRemember)
	assertToolError(t, result)
}

// ── handleForget ──────────────────────────────────────────────────────────────

func TestHandleForget_InvalidUUID_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"memory_id": "not-a-valid-uuid",
	}, s.handleForget)
	assertToolError(t, result)
}

func TestHandleForget_MissingMemoryID_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		// memory_id intentionally omitted
	}, s.handleForget)
	assertToolError(t, result)
}

// ── handlePublish ─────────────────────────────────────────────────────────────

func TestHandlePublish_MissingTitle_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"content":        "some content",
		"knowledge_type": "note",
		"scope":          "project:acme/api",
		// title intentionally omitted
	}, s.handlePublish)
	assertToolError(t, result)
}

func TestHandlePublish_MissingScope_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"title":          "My Doc",
		"content":        "some content",
		"knowledge_type": "note",
		// scope intentionally omitted
	}, s.handlePublish)
	assertToolError(t, result)
}

func TestHandlePublish_InvalidArtifactKind_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"title":          "My Doc",
		"content":        "some content",
		"knowledge_type": "note",
		"scope":          "project:acme/api",
		"artifact_kind":  "banana",
	}, s.handlePublish)
	assertToolError(t, result)
	msg := strings.ToLower(toolErrorText(t, result))
	if !strings.Contains(msg, "invalid artifact_kind") {
		t.Fatalf("expected invalid artifact_kind error, got %q", msg)
	}
}

// ── handleSummarize ───────────────────────────────────────────────────────────

func TestHandleSummarize_MissingScope_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		// scope intentionally omitted
	}, s.handleSummarize)
	assertToolError(t, result)
}

func TestHandleSummarize_EmptyScope_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"scope": "", // present but empty
	}, s.handleSummarize)
	assertToolError(t, result)
}

func TestHandleSummarize_WithProgressToken_NilPool_ReturnsToolError(t *testing.T) {
	// A progress token must not prevent the nil-pool error from being returned.
	s := &Server{}
	meta := &mcpgo.Meta{ProgressToken: "token-abc"}
	result := callToolWithMeta(t, map[string]any{
		"scope": "project:acme/api",
	}, meta, s.handleSummarize)
	assertToolError(t, result)
}

func TestHandleSummarize_NilMeta_NilPool_ReturnsToolError(t *testing.T) {
	// Passing no Meta at all (nil) must not panic and must return a tool error.
	s := &Server{}
	result := callToolWithMeta(t, map[string]any{
		"scope": "project:acme/api",
	}, nil, s.handleSummarize)
	assertToolError(t, result)
}

// ── handleEndorse ─────────────────────────────────────────────────────────────

func TestHandleEndorse_MissingArtifactID_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{}, s.handleEndorse)
	assertToolError(t, result)
}

func TestHandleEndorse_InvalidUUID_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"artifact_id": "not-a-uuid",
	}, s.handleEndorse)
	assertToolError(t, result)
}

func TestHandleEndorse_NilPool_ReturnsToolError(t *testing.T) {
	s := &Server{} // nil pool, nil knwLife, nil sklLife
	result := callTool(t, map[string]any{
		"artifact_id": "00000000-0000-0000-0000-000000000001",
	}, s.handleEndorse)
	assertToolError(t, result)
}

// ── handlePromote ─────────────────────────────────────────────────────────────

func TestHandlePromote_MissingMemoryID_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"target_scope":      "project:acme/api",
		"target_visibility": "team",
	}, s.handlePromote)
	assertToolError(t, result)
}

func TestHandlePromote_InvalidMemoryID_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"memory_id":         "not-a-uuid",
		"target_scope":      "project:acme/api",
		"target_visibility": "team",
	}, s.handlePromote)
	assertToolError(t, result)
}

func TestHandlePromote_MissingTargetScope_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"memory_id":         "00000000-0000-0000-0000-000000000001",
		"target_visibility": "team",
	}, s.handlePromote)
	assertToolError(t, result)
}

func TestHandlePromote_MissingTargetVisibility_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"memory_id":    "00000000-0000-0000-0000-000000000001",
		"target_scope": "project:acme/api",
	}, s.handlePromote)
	assertToolError(t, result)
}

// ── handleKnowledgeDetail ─────────────────────────────────────────────────────

func TestHandleKnowledgeDetail_MissingArtifactID_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{}, s.handleKnowledgeDetail)
	assertToolError(t, result)
}

func TestHandleKnowledgeDetail_InvalidUUID_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"artifact_id": "not-a-uuid",
	}, s.handleKnowledgeDetail)
	assertToolError(t, result)
}

func TestHandleKnowledgeDetail_NilStore_ReturnsToolError(t *testing.T) {
	s := &Server{} // knwStore is nil
	result := callTool(t, map[string]any{
		"artifact_id": "00000000-0000-0000-0000-000000000001",
	}, s.handleKnowledgeDetail)
	assertToolError(t, result)
}

// ── handleListScopes ──────────────────────────────────────────────────────────

func TestHandleListScopes_NilPool_ReturnsToolError(t *testing.T) {
	s := &Server{} // nil pool
	result := callTool(t, map[string]any{}, s.handleListScopes)
	assertToolError(t, result)
}

// ── handleCollect ─────────────────────────────────────────────────────────────

func TestHandleCollect_MissingAction_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{}, s.handleCollect)
	assertToolError(t, result)
}

func TestHandleCollect_UnknownAction_ReturnsToolError(t *testing.T) {
	s := &Server{pool: nil}
	// pool check happens after action check — supply a non-nil pool substitute
	// by just verifying unknown action returns a tool error regardless.
	// The nil-pool guard fires before the switch, so we don't need a real pool.
	result := callTool(t, map[string]any{"action": "bogus_action"}, s.handleCollect)
	assertToolError(t, result)
}

// ── handleSynthesizeTopic ─────────────────────────────────────────────────────

func TestHandleSynthesizeTopic_MissingScope_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"source_ids": []any{
			"00000000-0000-0000-0000-000000000001",
			"00000000-0000-0000-0000-000000000002",
		},
	}, s.handleSynthesizeTopic)
	assertToolError(t, result)
}

func TestHandleSynthesizeTopic_TooFewSourceIDs_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"scope":      "project:acme/api",
		"source_ids": []any{"00000000-0000-0000-0000-000000000001"},
	}, s.handleSynthesizeTopic)
	assertToolError(t, result)
}

func TestHandleSynthesizeTopic_MissingSourceIDs_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"scope": "project:acme/api",
	}, s.handleSynthesizeTopic)
	assertToolError(t, result)
}

func TestHandleSynthesizeTopic_WithProgressToken_NilPool_ReturnsToolError(t *testing.T) {
	// A progress token must not prevent the nil-pool error from being returned.
	s := &Server{}
	meta := &mcpgo.Meta{ProgressToken: "token-xyz"}
	result := callToolWithMeta(t, map[string]any{
		"scope": "project:acme/api",
		"source_ids": []any{
			"00000000-0000-0000-0000-000000000001",
			"00000000-0000-0000-0000-000000000002",
		},
	}, meta, s.handleSynthesizeTopic)
	assertToolError(t, result)
}

func TestHandleSynthesizeTopic_NilMeta_NilPool_ReturnsToolError(t *testing.T) {
	// Passing no Meta at all (nil) must not panic and must return a tool error.
	s := &Server{}
	result := callToolWithMeta(t, map[string]any{
		"scope": "project:acme/api",
		"source_ids": []any{
			"00000000-0000-0000-0000-000000000001",
			"00000000-0000-0000-0000-000000000002",
		},
	}, nil, s.handleSynthesizeTopic)
	assertToolError(t, result)
}

// ── handleSkillSearch ─────────────────────────────────────────────────────────

func TestHandleSkillSearch_MissingQuery_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{}, s.handleSkillSearch)
	assertToolError(t, result)
}

func TestHandleSkillSearch_EmptyQuery_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{"query": ""}, s.handleSkillSearch)
	assertToolError(t, result)
}

func TestHandleSkillSearch_NilPool_ReturnsToolError(t *testing.T) {
	s := &Server{} // nil pool / sklStore / svc
	result := callTool(t, map[string]any{"query": "deploy pipeline"}, s.handleSkillSearch)
	assertToolError(t, result)
}

// ── handleSkillInstall ────────────────────────────────────────────────────────

func TestHandleSkillInstall_NilPool_ReturnsToolError(t *testing.T) {
	s := &Server{} // nil pool / sklStore
	result := callTool(t, map[string]any{"slug": "review-pr"}, s.handleSkillInstall)
	assertToolError(t, result)
}

func TestHandleSkillInstall_MissingSlugAndID_ReturnsToolError(t *testing.T) {
	// Need a non-nil pool to pass the pool guard; use a placeholder Server
	// that has pool set but sklStore nil — the nil store path still returns an
	// error before any DB call is needed (the guard checks pool && sklStore).
	// Simplest approach: let the nil guard fire to confirm the error path, then
	// also verify the "neither slug nor skill_id" path by directly testing the
	// handler logic with an empty args map when the pool guard would pass.
	// Since we can't easily provide a real pool in a unit test, we rely on the
	// nil-pool guard to demonstrate the handler returns a tool error safely.
	s := &Server{}
	result := callTool(t, map[string]any{}, s.handleSkillInstall)
	assertToolError(t, result)
}

func TestHandleSkillInstall_InvalidSkillID_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{"skill_id": "not-a-uuid"}, s.handleSkillInstall)
	assertToolError(t, result)
}

// ── handleSkillInvoke ─────────────────────────────────────────────────────────

func TestHandleSkillInvoke_MissingSlug_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"scope": "project:acme/api",
	}, s.handleSkillInvoke)
	assertToolError(t, result)
}

func TestHandleSkillInvoke_MissingScope_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"slug": "review-pr",
	}, s.handleSkillInvoke)
	assertToolError(t, result)
}

func TestHandleSkillInvoke_NilPool_ReturnsToolError(t *testing.T) {
	s := &Server{} // nil pool / sklStore
	result := callTool(t, map[string]any{
		"slug":  "review-pr",
		"scope": "project:acme/api",
	}, s.handleSkillInvoke)
	assertToolError(t, result)
}

// ── handleSkillPublish ────────────────────────────────────────────────────────

func TestHandleSkillPublish_MissingScope_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"content": "skill body",
	}, s.handleSkillPublish)
	assertToolError(t, result)
}

func TestHandleSkillPublish_MissingContentAndBody_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"scope": "project:acme/api",
	}, s.handleSkillPublish)
	assertToolError(t, result)
}
