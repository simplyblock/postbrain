package mcp

import (
	"context"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/authz"
)

// authorizeToolPermission returns true if the token in ctx holds the required permission.
func authorizeToolPermission(ctx context.Context, required authz.Permission) bool {
	perms, _ := ctx.Value(auth.ContextKeyPermissions).(authz.PermissionSet)
	return perms.Contains(required)
}

func permissionToolError() *mcpgo.CallToolResult {
	return mcpgo.NewToolResultError("forbidden: insufficient permissions")
}

// withToolPermission wraps a tool handler, returning a permission error if the
// token in context does not hold the required authz.Permission.
func withToolPermission(required authz.Permission, fn mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		if !authorizeToolPermission(ctx, required) {
			return permissionToolError(), nil
		}
		return fn(ctx, req)
	}
}

// withAnyToolPermission wraps a tool handler that accepts any of the given permissions.
func withAnyToolPermission(required []authz.Permission, fn mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		perms, _ := ctx.Value(auth.ContextKeyPermissions).(authz.PermissionSet)
		for _, p := range required {
			if perms.Contains(p) {
				return fn(ctx, req)
			}
		}
		return permissionToolError(), nil
	}
}
