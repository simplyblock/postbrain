package mcp

import (
	"context"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
)

type toolPermission string

const (
	permissionNone  toolPermission = "none"
	permissionRead  toolPermission = "read"
	permissionWrite toolPermission = "write"
)

func authorizeToolPermission(ctx context.Context, required toolPermission) bool {
	if required == permissionNone {
		return true
	}
	token, _ := ctx.Value(auth.ContextKeyToken).(*db.Token)
	if token == nil {
		return false
	}
	switch required {
	case permissionRead:
		return auth.HasReadPermission(token.Permissions)
	case permissionWrite:
		return auth.HasWritePermission(token.Permissions)
	default:
		return false
	}
}

func permissionToolError() *mcpgo.CallToolResult {
	return mcpgo.NewToolResultError("forbidden: insufficient permissions")
}

func withToolPermission(required toolPermission, fn mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		if !authorizeToolPermission(ctx, required) {
			return permissionToolError(), nil
		}
		return fn(ctx, req)
	}
}
