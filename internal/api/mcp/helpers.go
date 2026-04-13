package mcp

import (
	"context"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/simplyblock/postbrain/internal/metrics"
)

// progressReporter returns a closure that sends notifications/progress
// notifications to the client. It is a no-op when the request carries no
// progress token or when the MCP server is nil.
func (s *Server) progressReporter(ctx context.Context, req mcpgo.CallToolRequest) func(progress, total float64, msg string) {
	var token mcpgo.ProgressToken
	if req.Params.Meta != nil {
		token = req.Params.Meta.ProgressToken
	}
	return func(progress, total float64, msg string) {
		if token == nil || s.mcpServer == nil {
			return
		}
		_ = s.mcpServer.SendNotificationToClient(ctx, "notifications/progress", map[string]any{
			"progressToken": token,
			"progress":      progress,
			"total":         total,
			"message":       msg,
		})
	}
}

// withToolMetrics wraps a ToolHandlerFunc to record the call duration in the
// postbrain_tool_duration_seconds histogram.
func withToolMetrics(toolName string, fn mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		start := time.Now()
		defer func() {
			metrics.ToolDuration.WithLabelValues(toolName).Observe(time.Since(start).Seconds())
		}()
		return fn(ctx, req)
	}
}

// argString returns the string value of key from args, or "" if absent or wrong type.
func argString(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}

// argIntOrDefault returns the integer value of key from args (JSON numbers arrive as float64),
// or def if the key is absent, zero, or negative.
func argIntOrDefault(args map[string]any, key string, def int) int {
	if v, ok := args[key].(float64); ok && v > 0 {
		return int(v)
	}
	return def
}

// argFloat64OrDefault returns the float64 value of key from args, or def if absent.
func argFloat64OrDefault(args map[string]any, key string, def float64) float64 {
	if v, ok := args[key].(float64); ok {
		return v
	}
	return def
}

// argBool returns the bool value of key from args, or false if absent or wrong type.
func argBool(args map[string]any, key string) bool {
	v, _ := args[key].(bool)
	return v
}

// argStringSlice returns a []string from a []any value at key, filtering non-string elements.
func argStringSlice(args map[string]any, key string) []string {
	v, ok := args[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(v))
	for _, item := range v {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
