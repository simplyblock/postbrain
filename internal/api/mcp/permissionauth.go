package mcp

import (
	"context"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/authz"
)

// contextKeyToolPermission stores the required permission for the current MCP tool
// in context, so scope authorization checks inside handlers can use it.
type contextKeyToolPermission struct{}

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
// It also stores the required permission in context for downstream scope checks.
func withToolPermission(required authz.Permission, fn mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		if !authorizeToolPermission(ctx, required) {
			return permissionToolError(), nil
		}
		ctx = context.WithValue(ctx, contextKeyToolPermission{}, required)
		return fn(ctx, req)
	}
}

// withAnyToolPermission wraps a tool handler that accepts any of the given permissions.
// It stores the first matched permission in context for downstream scope checks.
func withAnyToolPermission(required []authz.Permission, fn mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		perms, _ := ctx.Value(auth.ContextKeyPermissions).(authz.PermissionSet)
		for _, p := range required {
			if perms.Contains(p) {
				ctx = context.WithValue(ctx, contextKeyToolPermission{}, p)
				return fn(ctx, req)
			}
		}
		return permissionToolError(), nil
	}
}
