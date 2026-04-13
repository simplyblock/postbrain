package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/db/compat"
)

func (s *Server) registerForget() {
	s.mcpServer.AddTool(mcpgo.NewTool("forget",
		mcpgo.WithReadOnlyHintAnnotation(false),
		mcpgo.WithDestructiveHintAnnotation(true),
		mcpgo.WithOpenWorldHintAnnotation(false),
		mcpgo.WithDescription("Deactivate or permanently delete a memory"),
		mcpgo.WithString("memory_id", mcpgo.Required(), mcpgo.Description("UUID of the memory to delete")),
		mcpgo.WithBoolean("hard", mcpgo.Description("true = permanent delete, false = soft-delete (default: false)")),
	), withToolMetrics("forget", withToolPermission("memories:delete", s.handleForget)))
}

// handleForget deactivates or permanently deletes a memory.
func (s *Server) handleForget(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	memIDStr := argString(args, "memory_id")
	if memIDStr == "" {
		return mcpgo.NewToolResultError("forget: 'memory_id' is required"), nil
	}

	memID, err := uuid.Parse(memIDStr)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("forget: invalid memory_id: %v", err)), nil
	}

	hard := argBool(args, "hard")

	if s.memStore == nil {
		return mcpgo.NewToolResultError("forget: server not configured (no memory store)"), nil
	}
	mem, err := compat.GetMemory(ctx, s.pool, memID)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("forget: lookup memory failed: %v", err)), nil
	}
	if mem == nil {
		return mcpgo.NewToolResultError("forget: memory not found"), nil
	}
	if err := s.authorizeDeleteObjectScope(ctx, mem.ScopeID); err != nil {
		return scopeAuthzToolError(ctx, "forget", mem.ScopeID, err), nil
	}

	action := "deactivated"
	if hard {
		if err := s.memStore.HardDelete(ctx, memID); err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("forget: hard delete failed: %v", err)), nil
		}
		action = "deleted"
	} else {
		if err := s.memStore.SoftDelete(ctx, memID); err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("forget: soft delete failed: %v", err)), nil
		}
	}

	out, _ := json.Marshal(map[string]any{
		"memory_id": memIDStr,
		"action":    action,
	})
	return mcpgo.NewToolResultText(string(out)), nil
}
