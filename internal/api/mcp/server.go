// Package mcp exposes Postbrain as a Model Context Protocol (MCP) server.
package mcp

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/embedding"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/memory"
	"github.com/simplyblock/postbrain/internal/metrics"
	"github.com/simplyblock/postbrain/internal/principals"
	"github.com/simplyblock/postbrain/internal/skills"
)

// Server wraps the MCP server with Postbrain dependencies.
type Server struct {
	mcpServer  *mcpserver.MCPServer
	pool       *pgxpool.Pool
	svc        *embedding.EmbeddingService
	tokenStore *auth.TokenStore
	cfg        *config.Config
	// layer stores
	memStore   *memory.Store
	knwStore   *knowledge.Store
	sklStore   *skills.Store
	knwLife    *knowledge.Lifecycle
	sklLife    *skills.Lifecycle
	knwColl    *knowledge.CollectionStore
	knwProm    *knowledge.Promoter
	membership *principals.MembershipStore
}

// NewServer creates all stores, registers all 13 tools, and returns the Server.
func NewServer(pool *pgxpool.Pool, svc *embedding.EmbeddingService, cfg *config.Config) *Server {
	s := &Server{
		pool: pool,
		svc:  svc,
		cfg:  cfg,
	}

	if pool != nil {
		s.tokenStore = auth.NewTokenStore(pool)
		s.memStore = memory.NewStore(pool, svc)
		s.knwStore = knowledge.NewStore(pool, svc)
		s.sklStore = skills.NewStore(pool, svc)
		s.membership = principals.NewMembershipStore(pool)
		s.knwLife = knowledge.NewLifecycle(pool, s.membership)
		s.sklLife = skills.NewLifecycle(pool, s.membership)
		s.knwColl = knowledge.NewCollectionStore(pool)
		s.knwProm = knowledge.NewPromoter(pool, svc)
	}

	s.mcpServer = mcpserver.NewMCPServer("postbrain", "1.0.0")
	s.registerTools()
	return s
}

// MCPServer returns the underlying mcp-go server, for use in tests.
func (s *Server) MCPServer() *mcpserver.MCPServer {
	return s.mcpServer
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

// registerTools registers all MCP tools.
func (s *Server) registerTools() {
	// remember
	s.mcpServer.AddTool(mcpgo.NewTool("remember",
		mcpgo.WithDescription("Store a new memory or update an existing near-duplicate"),
		mcpgo.WithString("content", mcpgo.Required(), mcpgo.Description("The memory content")),
		mcpgo.WithString("scope", mcpgo.Required(), mcpgo.Description("Target scope as kind:external_id, e.g. project:acme/api")),
		mcpgo.WithString("memory_type", mcpgo.Description("semantic|episodic|procedural|working (default: semantic)")),
		mcpgo.WithNumber("importance", mcpgo.Description("Importance score 0–1 (default: 0.5)")),
		mcpgo.WithString("summary", mcpgo.Description("Optional memory summary")),
		mcpgo.WithString("source_ref", mcpgo.Description("Provenance reference, e.g. file:src/main.go:42")),
		mcpgo.WithArray("entities", mcpgo.Description("Entities to link. Each item is an object with 'name' (canonical string) and 'type' (concept|technology|file|person|service|pr|decision). Bare strings are accepted for backwards compatibility and default to type 'concept'.")),
		mcpgo.WithNumber("expires_in", mcpgo.Description("TTL in seconds; only for memory_type=working")),
	), withToolMetrics("remember", s.handleRemember))

	// recall
	s.mcpServer.AddTool(mcpgo.NewTool("recall",
		mcpgo.WithDescription("Retrieve memories and knowledge relevant to a query"),
		mcpgo.WithString("query", mcpgo.Required(), mcpgo.Description("Semantic search query")),
		mcpgo.WithString("scope", mcpgo.Required(), mcpgo.Description("Scope as kind:external_id")),
		mcpgo.WithArray("memory_types", mcpgo.Description("Filter by memory type: semantic|episodic|procedural|working")),
		mcpgo.WithArray("layers", mcpgo.Description("Layers to query: memory|knowledge|skill (default: all)")),
		mcpgo.WithString("agent_type", mcpgo.Description("Filter skills by agent compatibility")),
		mcpgo.WithNumber("limit", mcpgo.Description("Max results (default: 10)")),
		mcpgo.WithNumber("min_score", mcpgo.Description("Min combined score 0–1 (default: 0.0)")),
		mcpgo.WithString("search_mode", mcpgo.Description("text|code|hybrid (default: hybrid)")),
		mcpgo.WithNumber("graph_depth", mcpgo.Description("Graph traversal depth for code results: 0=off, 1=direct neighbours (default: 1)")),
	), withToolMetrics("recall", s.handleRecall))

	// forget
	s.mcpServer.AddTool(mcpgo.NewTool("forget",
		mcpgo.WithDescription("Deactivate or permanently delete a memory"),
		mcpgo.WithString("memory_id", mcpgo.Required(), mcpgo.Description("UUID of the memory to delete")),
		mcpgo.WithBoolean("hard", mcpgo.Description("true = permanent delete, false = soft-delete (default: false)")),
	), withToolMetrics("forget", s.handleForget))

	// context
	s.mcpServer.AddTool(mcpgo.NewTool("context",
		mcpgo.WithDescription("Retrieve a context bundle for the current scope and query"),
		mcpgo.WithString("scope", mcpgo.Required(), mcpgo.Description("Scope as kind:external_id")),
		mcpgo.WithString("query", mcpgo.Description("What you are about to work on")),
		mcpgo.WithNumber("max_tokens", mcpgo.Description("Token budget for context (default: 4000)")),
	), withToolMetrics("context", s.handleContext))

	// summarize
	s.mcpServer.AddTool(mcpgo.NewTool("summarize",
		mcpgo.WithDescription("Consolidate memories into a higher-level semantic memory"),
		mcpgo.WithString("scope", mcpgo.Required(), mcpgo.Description("Scope as kind:external_id")),
		mcpgo.WithString("topic", mcpgo.Description("Topic to cluster and summarize")),
		mcpgo.WithBoolean("dry_run", mcpgo.Description("If true, preview without writing (default: false)")),
	), withToolMetrics("summarize", s.handleSummarize))

	// publish
	s.mcpServer.AddTool(mcpgo.NewTool("publish",
		mcpgo.WithDescription("Create or update a knowledge artifact"),
		mcpgo.WithString("title", mcpgo.Required(), mcpgo.Description("Artifact title")),
		mcpgo.WithString("content", mcpgo.Required(), mcpgo.Description("Artifact content")),
		mcpgo.WithString("knowledge_type", mcpgo.Required(), mcpgo.Description("semantic|episodic|procedural|reference")),
		mcpgo.WithString("scope", mcpgo.Required(), mcpgo.Description("Owner scope as kind:external_id")),
		mcpgo.WithString("visibility", mcpgo.Description("private|project|team|department|company (default: team)")),
		mcpgo.WithString("summary", mcpgo.Description("Short summary")),
		mcpgo.WithBoolean("auto_review", mcpgo.Description("Move directly to in_review (default: false)")),
		mcpgo.WithString("collection_slug", mcpgo.Description("Add to this collection slug after creation")),
	), withToolMetrics("publish", s.handlePublish))

	// endorse
	s.mcpServer.AddTool(mcpgo.NewTool("endorse",
		mcpgo.WithDescription("Endorse a knowledge artifact or skill"),
		mcpgo.WithString("artifact_id", mcpgo.Required(), mcpgo.Description("UUID of the artifact or skill to endorse")),
		mcpgo.WithString("note", mcpgo.Description("Optional endorsement note")),
	), withToolMetrics("endorse", s.handleEndorse))

	// promote
	s.mcpServer.AddTool(mcpgo.NewTool("promote",
		mcpgo.WithDescription("Nominate a memory for elevation into a knowledge artifact"),
		mcpgo.WithString("memory_id", mcpgo.Required(), mcpgo.Description("UUID of the memory to promote")),
		mcpgo.WithString("target_scope", mcpgo.Required(), mcpgo.Description("Target scope as kind:external_id")),
		mcpgo.WithString("target_visibility", mcpgo.Required(), mcpgo.Description("Visibility level")),
		mcpgo.WithString("proposed_title", mcpgo.Description("Proposed title for the knowledge artifact")),
		mcpgo.WithString("collection_slug", mcpgo.Description("Optionally add to this collection slug")),
	), withToolMetrics("promote", s.handlePromote))

	// collect
	s.mcpServer.AddTool(mcpgo.NewTool("collect",
		mcpgo.WithDescription("Add artifact to collection, create collection, or list collections"),
		mcpgo.WithString("action", mcpgo.Required(), mcpgo.Description("add_to_collection|create_collection|list_collections")),
		mcpgo.WithString("artifact_id", mcpgo.Description("UUID of the artifact (for add_to_collection)")),
		mcpgo.WithString("collection_id", mcpgo.Description("UUID of the collection (for add_to_collection)")),
		mcpgo.WithString("collection_slug", mcpgo.Description("Slug alternative to collection_id")),
		mcpgo.WithString("scope", mcpgo.Description("Scope as kind:external_id (required for create_collection and when using collection_slug)")),
		mcpgo.WithString("name", mcpgo.Description("Collection name (required for create_collection)")),
		mcpgo.WithString("description", mcpgo.Description("Collection description (optional)")),
	), withToolMetrics("collect", s.handleCollect))

	// skill_search
	s.mcpServer.AddTool(mcpgo.NewTool("skill_search",
		mcpgo.WithDescription("Search for skills by semantic similarity"),
		mcpgo.WithString("query", mcpgo.Required(), mcpgo.Description("Search query")),
		mcpgo.WithString("scope", mcpgo.Description("Scope as kind:external_id")),
		mcpgo.WithString("agent_type", mcpgo.Description("Filter by agent compatibility")),
		mcpgo.WithNumber("limit", mcpgo.Description("Max results (default: 10)")),
		mcpgo.WithBoolean("installed", mcpgo.Description("Filter by installed status")),
	), withToolMetrics("skill_search", s.handleSkillSearch))

	// skill_install
	s.mcpServer.AddTool(mcpgo.NewTool("skill_install",
		mcpgo.WithDescription("Materialise a skill into the agent command directory"),
		mcpgo.WithString("skill_id", mcpgo.Description("UUID of the skill to install")),
		mcpgo.WithString("slug", mcpgo.Description("Slug alternative to skill_id")),
		mcpgo.WithString("scope", mcpgo.Description("Scope as kind:external_id")),
		mcpgo.WithString("agent_type", mcpgo.Description("Target agent type")),
		mcpgo.WithString("workdir", mcpgo.Description("Working directory for installation")),
	), withToolMetrics("skill_install", s.handleSkillInstall))

	// skill_invoke
	s.mcpServer.AddTool(mcpgo.NewTool("skill_invoke",
		mcpgo.WithDescription("Look up a skill by slug, substitute params, return expanded body"),
		mcpgo.WithString("slug", mcpgo.Required(), mcpgo.Description("Skill slug")),
		mcpgo.WithString("scope", mcpgo.Required(), mcpgo.Description("Scope as kind:external_id")),
		mcpgo.WithString("agent_type", mcpgo.Description("Agent type for filtering")),
		mcpgo.WithObject("params", mcpgo.Description("Parameter map for substitution")),
		mcpgo.WithString("session_id", mcpgo.Description("Session ID from session_begin; used to correlate invocation events")),
	), withToolMetrics("skill_invoke", s.handleSkillInvoke))

	// knowledge_detail
	s.mcpServer.AddTool(mcpgo.NewTool("knowledge_detail",
		mcpgo.WithDescription("Retrieve the full content of a knowledge artifact by ID. Use when recall returns full_content_available=true and the summary is insufficient."),
		mcpgo.WithString("artifact_id", mcpgo.Required(), mcpgo.Description("UUID of the knowledge artifact")),
	), withToolMetrics("knowledge_detail", s.handleKnowledgeDetail))

	// list_scopes
	s.mcpServer.AddTool(mcpgo.NewTool("list_scopes",
		mcpgo.WithDescription("List all scopes accessible to the current token. Returns scope IDs and their kind:external_id strings for use in other tools."),
	), withToolMetrics("list_scopes", s.handleListScopes))

	// session_begin
	s.mcpServer.AddTool(mcpgo.NewTool("session_begin",
		mcpgo.WithDescription("Start a new agent session for a scope. Returns a session_id to pass to skill_invoke for event correlation. Call once at the start of each agent session."),
		mcpgo.WithString("scope", mcpgo.Required(), mcpgo.Description("Scope as kind:external_id")),
	), withToolMetrics("session_begin", s.handleSessionBegin))

	// session_end
	s.mcpServer.AddTool(mcpgo.NewTool("session_end",
		mcpgo.WithDescription("Close an agent session. Call when the agent session is ending (e.g. in a Stop hook)."),
		mcpgo.WithString("session_id", mcpgo.Required(), mcpgo.Description("Session ID returned by session_begin")),
	), withToolMetrics("session_end", s.handleSessionEnd))

	// synthesize_topic
	s.mcpServer.AddTool(mcpgo.NewTool("synthesize_topic",
		mcpgo.WithDescription("Synthesise multiple published knowledge artifacts into a single topic digest artifact"),
		mcpgo.WithString("scope", mcpgo.Required(), mcpgo.Description("Owner scope as kind:external_id")),
		mcpgo.WithArray("source_ids", mcpgo.Required(), mcpgo.Description("UUIDs of the source artifacts to synthesise (minimum 2, all must be published non-digest artifacts)")),
		mcpgo.WithString("title", mcpgo.Description("Digest title; inferred from sources if omitted")),
		mcpgo.WithBoolean("auto_review", mcpgo.Description("Move directly to in_review (default: false)")),
	), withToolMetrics("synthesize_topic", s.handleSynthesizeTopic))
}

// Handler returns an http.Handler that serves both the MCP Streamable HTTP
// transport (used by Codex and other modern clients) and the legacy SSE
// transport (used by Claude Code), both at the same mount path.
func (s *Server) Handler() http.Handler {
	streamable := mcpserver.NewStreamableHTTPServer(s.mcpServer)
	sseServer := mcpserver.NewSSEServer(s.mcpServer)

	mux := http.NewServeMux()
	mux.Handle("/sse", sseServer)
	mux.Handle("/message", sseServer)
	mux.Handle("/", streamable)

	var h http.Handler = mux
	if s.tokenStore != nil && s.pool != nil {
		mw := auth.BearerTokenMiddleware(s.tokenStore, s.pool)
		h = mw(mux)
	}
	return h
}

// parseScopeString splits a scope string of the form "kind:external_id" into its parts.
// Returns an error if the string is empty or missing the colon separator.
func parseScopeString(scope string) (kind, externalID string, err error) {
	if scope == "" {
		return "", "", errorString("scope: empty scope string")
	}
	idx := strings.Index(scope, ":")
	if idx < 0 {
		return "", "", errorString("scope: missing ':' separator in scope string: " + scope)
	}
	return scope[:idx], scope[idx+1:], nil
}

// errorString is a simple error implementation to avoid importing "errors" just for this.
type errorString string

func (e errorString) Error() string { return string(e) }
