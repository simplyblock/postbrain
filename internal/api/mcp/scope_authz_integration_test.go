//go:build integration

package mcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	mcpapi "github.com/simplyblock/postbrain/internal/api/mcp"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/authz"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/principals"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestMCP_ScopeAuthz_ScopeTakingTools(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "mcp-scope-authz-"+uuid.New().String())
	allowed := testhelper.CreateTestScope(t, pool, "project", "mcp-scope-allowed-"+uuid.New().String(), nil, principal.ID)
	blocked := testhelper.CreateTestScope(t, pool, "project", "mcp-scope-blocked-"+uuid.New().String(), nil, principal.ID)
	testhelper.CreateTestEmbeddingModel(t, pool)
	artifactA := testhelper.CreateTestArtifact(t, pool, allowed.ID, principal.ID, "scope authz source artifact a")
	artifactB := testhelper.CreateTestArtifact(t, pool, allowed.ID, principal.ID, "scope authz source artifact b")
	collectionSlug := "scope-authz-collection-" + uuid.New().String()
	_, err := db.CreateCollection(ctx, pool, &db.KnowledgeCollection{
		ScopeID:    allowed.ID,
		OwnerID:    principal.ID,
		Slug:       collectionSlug,
		Name:       "Scope Authz Collection",
		Visibility: "team",
	})
	if err != nil {
		t.Fatalf("create collection fixture: %v", err)
	}
	skillSlug := "scope-authz-skill-" + uuid.New().String()
	paramsJSON, err := json.Marshal([]db.SkillParameter{})
	if err != nil {
		t.Fatalf("marshal skill parameters: %v", err)
	}
	now := time.Now().UTC()
	_, err = db.CreateSkill(ctx, pool, &db.Skill{
		ScopeID:        allowed.ID,
		AuthorID:       principal.ID,
		Slug:           skillSlug,
		Name:           "Scope Authz Skill",
		Description:    "scope authz skill fixture",
		AgentTypes:     []string{"any"},
		Body:           "skill body",
		Parameters:     paramsJSON,
		Visibility:     "team",
		Status:         "published",
		PublishedAt:    &now,
		ReviewRequired: 1,
		Version:        1,
	})
	if err != nil {
		t.Fatalf("create skill fixture: %v", err)
	}

	srv := mcpapi.NewServer(pool, svc, cfg)
	mcpSrv := srv.MCPServer()

	scopeAllowed := "project:" + allowed.ExternalID
	scopeBlocked := "project:" + blocked.ExternalID
	ctxAuth := withAuthContext(ctx, pool, principal.ID, allowed.ID)
	promoteMemoryID := createMemoryViaRemember(t, mcpSrv, ctxAuth, scopeAllowed, "scope authz promotion memory")

	type toolCase struct {
		name    string
		argsFor func(scope string) map[string]any
	}
	cases := []toolCase{
		{
			name: "remember",
			argsFor: func(scope string) map[string]any {
				return map[string]any{
					"content":     "mcp scope authz remember content",
					"scope":       scope,
					"memory_type": "semantic",
				}
			},
		},
		{
			name: "publish",
			argsFor: func(scope string) map[string]any {
				return map[string]any{
					"title":          "mcp scope authz artifact",
					"content":        "artifact content",
					"knowledge_type": "semantic",
					"scope":          scope,
				}
			},
		},
		{
			name: "recall",
			argsFor: func(scope string) map[string]any {
				return map[string]any{
					"query":  "scope authz",
					"scope":  scope,
					"layers": []any{"memory"},
				}
			},
		},
		{
			name: "context",
			argsFor: func(scope string) map[string]any {
				return map[string]any{
					"scope": scope,
					"query": "scope authz",
				}
			},
		},
		{
			name: "skill_search",
			argsFor: func(scope string) map[string]any {
				return map[string]any{
					"query": "scope authz",
					"scope": scope,
				}
			},
		},
		{
			name: "promote",
			argsFor: func(scope string) map[string]any {
				return map[string]any{
					"memory_id":         promoteMemoryID.String(),
					"target_scope":      scope,
					"target_visibility": "team",
				}
			},
		},
		{
			name: "collect create_collection",
			argsFor: func(scope string) map[string]any {
				return map[string]any{
					"action":          "create_collection",
					"scope":           scope,
					"name":            "Scope Authz Created Collection",
					"collection_slug": "scope-authz-created-" + uuid.New().String(),
				}
			},
		},
		{
			name: "collect list_collections",
			argsFor: func(scope string) map[string]any {
				return map[string]any{
					"action": "list_collections",
					"scope":  scope,
				}
			},
		},
		{
			name: "collect add_to_collection via slug",
			argsFor: func(scope string) map[string]any {
				return map[string]any{
					"action":          "add_to_collection",
					"artifact_id":     artifactA.ID.String(),
					"collection_slug": collectionSlug,
					"scope":           scope,
				}
			},
		},
		{
			name: "session_begin",
			argsFor: func(scope string) map[string]any {
				return map[string]any{
					"scope": scope,
				}
			},
		},
		{
			name: "summarize",
			argsFor: func(scope string) map[string]any {
				return map[string]any{
					"scope":   scope,
					"topic":   "scope authz",
					"dry_run": true,
				}
			},
		},
		{
			name: "synthesize_topic",
			argsFor: func(scope string) map[string]any {
				return map[string]any{
					"scope": scope,
					"source_ids": []any{
						artifactA.ID.String(),
						artifactB.ID.String(),
					},
					"title": "Scope Authz Digest",
				}
			},
		},
		{
			name: "skill_install",
			argsFor: func(scope string) map[string]any {
				return map[string]any{
					"slug":       skillSlug,
					"scope":      scope,
					"agent_type": "codex",
					"workdir":    t.TempDir(),
				}
			},
		},
		{
			name: "skill_invoke",
			argsFor: func(scope string) map[string]any {
				return map[string]any{
					"slug":  skillSlug,
					"scope": scope,
				}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		toolName := firstWord(tc.name)
		tool := mcpSrv.GetTool(toolName)
		if tool == nil {
			t.Fatalf("tool %q not registered", tc.name)
		}

		t.Run(tc.name+" authorized scope succeeds", func(t *testing.T) {
			req := mcpgo.CallToolRequest{}
			req.Params.Name = toolName
			req.Params.Arguments = tc.argsFor(scopeAllowed)
			result, err := tool.Handler(ctxAuth, req)
			if err != nil {
				t.Fatalf("%s handler error: %v", tc.name, err)
			}
			if result == nil || result.IsError {
				t.Fatalf("%s expected success, got %+v", tc.name, result)
			}
		})

		t.Run(tc.name+" unauthorized scope is forbidden", func(t *testing.T) {
			req := mcpgo.CallToolRequest{}
			req.Params.Name = toolName
			req.Params.Arguments = tc.argsFor(scopeBlocked)
			result, err := tool.Handler(ctxAuth, req)
			if err != nil {
				t.Fatalf("%s handler error: %v", tc.name, err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("%s expected error result, got %+v", tc.name, result)
			}
			msg := firstToolText(result)
			if !strings.Contains(msg, "forbidden: scope access denied") {
				t.Fatalf("%s error text = %q, want forbidden scope access denied", tc.name, msg)
			}
		})

		t.Run(tc.name+" malformed scope returns invalid scope error", func(t *testing.T) {
			req := mcpgo.CallToolRequest{}
			req.Params.Name = toolName
			req.Params.Arguments = tc.argsFor("invalid-scope-format")
			result, err := tool.Handler(ctxAuth, req)
			if err != nil {
				t.Fatalf("%s handler error: %v", tc.name, err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("%s expected error result, got %+v", tc.name, result)
			}
			msg := firstToolText(result)
			if !strings.Contains(msg, "invalid scope") && !strings.Contains(msg, "invalid target_scope") {
				t.Fatalf("%s error text = %q, want invalid scope", tc.name, msg)
			}
		})
	}
}

func firstWord(s string) string {
	idx := strings.IndexByte(s, ' ')
	if idx < 0 {
		return s
	}
	return s[:idx]
}

func createMemoryViaRemember(t *testing.T, mcpSrv *mcpserver.MCPServer, ctx context.Context, scope, content string) uuid.UUID {
	t.Helper()
	rememberTool := mcpSrv.GetTool("remember")
	if rememberTool == nil {
		t.Fatal("remember tool not registered")
	}

	req := mcpgo.CallToolRequest{}
	req.Params.Name = "remember"
	req.Params.Arguments = map[string]any{
		"content":     content,
		"scope":       scope,
		"memory_type": "semantic",
	}
	result, err := rememberTool.Handler(ctx, req)
	if err != nil {
		t.Fatalf("remember fixture handler error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("remember fixture failed: %+v", result)
	}
	text := firstToolText(result)
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("remember fixture invalid JSON %q: %v", text, err)
	}
	memIDStr, _ := out["memory_id"].(string)
	memID, err := uuid.Parse(memIDStr)
	if err != nil {
		t.Fatalf("remember fixture invalid memory_id %q: %v", memIDStr, err)
	}
	return memID
}

func firstToolText(result *mcpgo.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	text, _ := result.Content[0].(mcpgo.TextContent)
	return text.Text
}

func TestMCP_ScopeAuthz_MultiHopChainMatrix(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}
	testhelper.CreateTestEmbeddingModel(t, pool)

	graph := testhelper.CreateScopeAuthzGraph(t, pool, "mcp-scope-multihop", "member")

	srv := mcpapi.NewServer(pool, svc, cfg)
	mcpSrv := srv.MCPServer()
	ctxAuth := withAuthContextUnrestricted(ctx, pool, graph.UserPrincipal.ID)

	allowedScopes := []string{
		"project:" + graph.UserScope.ExternalID,
		"project:" + graph.TeamScope.ExternalID,
		"project:" + graph.CompanyScope.ExternalID,
	}
	deniedScope := "project:" + graph.UnrelatedScope.ExternalID

	type toolCase struct {
		name    string
		argsFor func(scope string) map[string]any
	}
	cases := []toolCase{
		{
			name: "remember",
			argsFor: func(scope string) map[string]any {
				return map[string]any{
					"content":     "mcp chain matrix remember",
					"scope":       scope,
					"memory_type": "semantic",
				}
			},
		},
		{
			name: "publish",
			argsFor: func(scope string) map[string]any {
				return map[string]any{
					"title":          "mcp chain matrix artifact",
					"content":        "artifact content",
					"knowledge_type": "semantic",
					"scope":          scope,
				}
			},
		},
		{
			name: "recall",
			argsFor: func(scope string) map[string]any {
				return map[string]any{
					"query":  "chain matrix",
					"scope":  scope,
					"layers": []any{"memory"},
				}
			},
		},
		{
			name: "context",
			argsFor: func(scope string) map[string]any {
				return map[string]any{
					"scope": scope,
					"query": "chain matrix",
				}
			},
		},
		{
			name: "skill_search",
			argsFor: func(scope string) map[string]any {
				return map[string]any{
					"query": "chain matrix",
					"scope": scope,
				}
			},
		},
		{
			name: "session_begin",
			argsFor: func(scope string) map[string]any {
				return map[string]any{"scope": scope}
			},
		},
		{
			name: "summarize",
			argsFor: func(scope string) map[string]any {
				return map[string]any{
					"scope":   scope,
					"dry_run": true,
					"topic":   "chain matrix",
				}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		tool := mcpSrv.GetTool(tc.name)
		if tool == nil {
			t.Fatalf("tool %q not registered", tc.name)
		}

		for _, scopeStr := range allowedScopes {
			scopeStr := scopeStr
			t.Run(tc.name+" allows "+scopeStr, func(t *testing.T) {
				req := mcpgo.CallToolRequest{}
				req.Params.Name = tc.name
				req.Params.Arguments = tc.argsFor(scopeStr)
				result, err := tool.Handler(ctxAuth, req)
				if err != nil {
					t.Fatalf("%s handler error: %v", tc.name, err)
				}
				if result == nil || result.IsError {
					t.Fatalf("%s expected success for scope %s, got %+v", tc.name, scopeStr, result)
				}
			})
		}

		t.Run(tc.name+" denies unrelated branch", func(t *testing.T) {
			req := mcpgo.CallToolRequest{}
			req.Params.Name = tc.name
			req.Params.Arguments = tc.argsFor(deniedScope)
			result, err := tool.Handler(ctxAuth, req)
			if err != nil {
				t.Fatalf("%s handler error: %v", tc.name, err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("%s expected error result, got %+v", tc.name, result)
			}
			msg := firstToolText(result)
			if !strings.Contains(msg, "forbidden: scope access denied") {
				t.Fatalf("%s error text = %q, want forbidden scope access denied", tc.name, msg)
			}
		})
	}
}

func withAuthContextUnrestricted(ctx context.Context, pool *pgxpool.Pool, principalID uuid.UUID) context.Context {
	rawPerms := []string{"read", "write", "edit", "delete"}
	perms, _ := authz.ParseTokenPermissions(rawPerms)
	ctx = context.WithValue(ctx, auth.ContextKeyPrincipalID, principalID)
	token := &db.Token{
		PrincipalID: principalID,
		ScopeIds:    nil,
		Permissions: rawPerms,
	}
	ctx = context.WithValue(ctx, auth.ContextKeyToken, token)
	ctx = context.WithValue(ctx, auth.ContextKeyPermissions, perms)
	if pool != nil {
		resolver := authz.NewTokenResolver(authz.NewDBResolver(pool))
		ctx = context.WithValue(ctx, auth.ContextKeyTokenResolver, resolver)
	}
	return ctx
}

func TestMCP_ScopeAuthz_ForgetWriteParentAllowedDeleteParentDenied(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	parentPrincipal := testhelper.CreateTestPrincipal(t, pool, "team", "mcp-delete-parent-team-"+uuid.NewString())
	childPrincipal := testhelper.CreateTestPrincipal(t, pool, "user", "mcp-delete-parent-user-"+uuid.NewString())

	parentScope := testhelper.CreateTestScope(t, pool, "project", "mcp-delete-parent-scope-"+uuid.NewString(), nil, parentPrincipal.ID)
	childScope := testhelper.CreateTestScope(t, pool, "project", "mcp-delete-child-scope-"+uuid.NewString(), nil, childPrincipal.ID)

	ms := principals.NewMembershipStore(pool)
	if err := ms.AddMembership(ctx, childPrincipal.ID, parentPrincipal.ID, "member", nil); err != nil {
		t.Fatalf("add membership child->parent: %v", err)
	}

	parentMemory := testhelper.CreateTestMemory(t, pool, parentScope.ID, parentPrincipal.ID, "parent memory for mcp forget")
	childMemory := testhelper.CreateTestMemory(t, pool, childScope.ID, childPrincipal.ID, "child memory for mcp forget")
	testhelper.CreateTestEmbeddingModel(t, pool)

	srv := mcpapi.NewServer(pool, svc, cfg).MCPServer()
	ctxAuth := withAuthContextUnrestricted(ctx, pool, childPrincipal.ID)

	rememberTool := srv.GetTool("remember")
	if rememberTool == nil {
		t.Fatal("remember tool not registered")
	}
	t.Run("write to parent scope succeeds", func(t *testing.T) {
		req := mcpgo.CallToolRequest{}
		req.Params.Name = "remember"
		req.Params.Arguments = map[string]any{
			"content":     "write into parent via mcp",
			"scope":       "project:" + parentScope.ExternalID,
			"memory_type": "semantic",
		}
		result, err := rememberTool.Handler(ctxAuth, req)
		if err != nil {
			t.Fatalf("remember handler error: %v", err)
		}
		if result == nil || result.IsError {
			t.Fatalf("remember expected success, got %+v", result)
		}
	})

	forgetTool := srv.GetTool("forget")
	if forgetTool == nil {
		t.Fatal("forget tool not registered")
	}
	t.Run("delete in own scope succeeds", func(t *testing.T) {
		req := mcpgo.CallToolRequest{}
		req.Params.Name = "forget"
		req.Params.Arguments = map[string]any{
			"memory_id": childMemory.ID.String(),
		}
		result, err := forgetTool.Handler(ctxAuth, req)
		if err != nil {
			t.Fatalf("forget handler error: %v", err)
		}
		if result == nil || result.IsError {
			t.Fatalf("forget expected success, got %+v", result)
		}
	})

	t.Run("delete in parent scope is forbidden", func(t *testing.T) {
		req := mcpgo.CallToolRequest{}
		req.Params.Name = "forget"
		req.Params.Arguments = map[string]any{
			"memory_id": parentMemory.ID.String(),
		}
		result, err := forgetTool.Handler(ctxAuth, req)
		if err != nil {
			t.Fatalf("forget handler error: %v", err)
		}
		if result == nil || !result.IsError {
			t.Fatalf("forget expected error result, got %+v", result)
		}
		msg := firstToolText(result)
		if !strings.Contains(msg, "forbidden: scope access denied") {
			t.Fatalf("forget error text = %q, want forbidden scope access denied", msg)
		}
	})
}
