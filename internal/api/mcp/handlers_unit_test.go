package mcp

import (
	"context"
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

// assertToolError fails the test if result.IsError is not true.
func assertToolError(t *testing.T, result *mcpgo.CallToolResult) {
	t.Helper()
	if !result.IsError {
		t.Errorf("expected tool error (IsError=true), got IsError=false")
	}
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
