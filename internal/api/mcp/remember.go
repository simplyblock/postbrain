package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/memory"
)

func (s *Server) registerRemember() {
	s.mcpServer.AddTool(mcpgo.NewTool("remember",
		mcpgo.WithReadOnlyHintAnnotation(false),
		mcpgo.WithDestructiveHintAnnotation(false),
		mcpgo.WithOpenWorldHintAnnotation(false),
		mcpgo.WithDescription("Store a new memory or update an existing near-duplicate"),
		mcpgo.WithString("content", mcpgo.Required(), mcpgo.Description("The memory content")),
		mcpgo.WithString("scope", mcpgo.Required(), mcpgo.Description("Target scope as kind:external_id, e.g. project:acme/api")),
		mcpgo.WithString("memory_type", mcpgo.Description("semantic|episodic|procedural|working (default: semantic)")),
		mcpgo.WithNumber("importance", mcpgo.Description("Importance score 0–1 (default: 0.5)")),
		mcpgo.WithString("summary", mcpgo.Description("Optional memory summary")),
		mcpgo.WithString("source_ref", mcpgo.Description("Provenance reference, e.g. file:src/main.go:42")),
		mcpgo.WithArray("entities",
			mcpgo.Items(map[string]any{
				"anyOf": []any{
					map[string]any{"type": "string"},
					map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name": map[string]any{"type": "string", "minLength": 1},
							"type": map[string]any{"type": "string"},
						},
						"required": []string{"name"},
					},
				},
			}),
			mcpgo.Description("Entities to link. Each item is an object with 'name' (canonical string) and 'type' (concept|technology|file|person|service|pr|decision). Bare strings are accepted for backwards compatibility and default to type 'concept'."),
		),
		mcpgo.WithNumber("expires_in", mcpgo.Description("TTL in seconds; only for memory_type=working")),
	), withToolMetrics("remember", withToolPermission("memories:write", s.handleRemember)))
}

// handleRemember stores a new memory or updates a near-duplicate.
func (s *Server) handleRemember(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	// Required: content
	content, ok := args["content"].(string)
	if !ok || content == "" {
		return mcpgo.NewToolResultError("remember: 'content' is required and must be a non-empty string"), nil
	}

	// Required: scope
	scopeStr, ok := args["scope"].(string)
	if !ok || scopeStr == "" {
		return mcpgo.NewToolResultError("remember: 'scope' is required"), nil
	}

	if s.pool == nil {
		return mcpgo.NewToolResultError("remember: server not configured (no database connection)"), nil
	}

	scopeID, errResult := s.resolveScope(ctx, "remember", scopeStr)
	if errResult != nil {
		return errResult, nil
	}

	// Get principal from context.
	principalID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	// Optional fields with defaults.
	memoryType := argString(args, "memory_type")
	if memoryType == "" {
		memoryType = "semantic"
	}
	importance := argFloat64OrDefault(args, "importance", 0.5)

	var sourceRef *string
	if v := argString(args, "source_ref"); v != "" {
		sourceRef = &v
	}
	var summary *string
	if v := argString(args, "summary"); v != "" {
		summary = &v
	}

	var entities []memory.EntityInput
	if v, ok := args["entities"].([]any); ok {
		for _, e := range v {
			switch val := e.(type) {
			case map[string]any:
				name, _ := val["name"].(string)
				typ, _ := val["type"].(string)
				if name != "" {
					entities = append(entities, memory.EntityInput{Name: name, Type: typ})
				}
			case string:
				if val != "" {
					entities = append(entities, memory.EntityInput{Name: val})
				}
			}
		}
	}

	var expiresIn *int
	if v, ok := args["expires_in"].(float64); ok && v > 0 {
		n := int(v)
		expiresIn = &n
	}

	input := memory.CreateInput{
		Content:    content,
		Summary:    summary,
		MemoryType: memoryType,
		ScopeID:    scopeID,
		AuthorID:   principalID,
		Importance: importance,
		SourceRef:  sourceRef,
		Entities:   entities,
		ExpiresIn:  expiresIn,
	}

	result, err := s.memStore.Create(ctx, input)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("remember: store failed: %v", err)), nil
	}

	out, _ := json.Marshal(map[string]any{
		"memory_id": result.MemoryID.String(),
		"action":    result.Action,
	})
	return mcpgo.NewToolResultText(string(out)), nil
}
