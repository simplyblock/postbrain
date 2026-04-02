package mcp

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/api/scopeauth"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
)

func TestScopeAuthzToolError_LogsFieldsAndIncrementsMetric(t *testing.T) {
	t.Helper()

	principalID := uuid.New()
	tokenID := uuid.New()
	requestedScopeID := uuid.New()
	tool := "remember"

	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(orig) })

	ctx := context.Background()
	ctx = context.WithValue(ctx, auth.ContextKeyPrincipalID, principalID)
	ctx = context.WithValue(ctx, auth.ContextKeyToken, &db.Token{
		ID:          tokenID,
		PrincipalID: principalID,
	})

	result := scopeAuthzToolError(ctx, tool, requestedScopeID, scopeauth.ErrTokenScopeDenied)
	if result == nil || !result.IsError {
		t.Fatalf("expected error result, got %+v", result)
	}
	msg := firstToolTextContent(result)
	if !strings.Contains(msg, "forbidden: scope access denied") {
		t.Fatalf("msg = %q, want forbidden: scope access denied", msg)
	}

	logText := buf.String()
	for _, fragment := range []string{
		"scope access denied",
		"surface=mcp",
		"endpoint=remember",
		"principal_id=" + principalID.String(),
		"token_id=" + tokenID.String(),
		"requested_scope_id=" + requestedScopeID.String(),
	} {
		if !strings.Contains(logText, fragment) {
			t.Fatalf("log missing %q in %q", fragment, logText)
		}
	}
}

func firstToolTextContent(result *mcpgo.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	text, _ := result.Content[0].(mcpgo.TextContent)
	return text.Text
}
