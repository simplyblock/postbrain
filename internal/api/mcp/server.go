// Package mcp exposes Postbrain as a Model Context Protocol (MCP) server.
package mcp

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/memory"
	"github.com/simplyblock/postbrain/internal/principals"
	"github.com/simplyblock/postbrain/internal/providers"
	"github.com/simplyblock/postbrain/internal/scopeutil"
	"github.com/simplyblock/postbrain/internal/skills"
)

// Server wraps the MCP server with Postbrain dependencies.
type Server struct {
	mcpServer  *mcpserver.MCPServer
	pool       *pgxpool.Pool
	svc        *providers.EmbeddingService
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
	ageEnabled bool
}

// MCPServer returns the underlying mcp-go server, for use in tests.
func (s *Server) MCPServer() *mcpserver.MCPServer {
	return s.mcpServer
}

// registerTools registers all MCP tools by delegating to per-tool register methods.
func (s *Server) registerTools() {
	s.registerRemember()
	s.registerRecall()
	s.registerCrossScopeContext()
	s.registerForget()
	s.registerContext()
	s.registerSummarize()
	s.registerPublish()
	s.registerEndorse()
	s.registerPromote()
	s.registerCollect()
	s.registerKnowledgeDetail()
	s.registerSkillSearch()
	s.registerSkillInstall()
	s.registerSkillInvoke()
	s.registerListScopes()
	s.registerSessionBegin()
	s.registerSessionEnd()
	s.registerSynthesizeTopic()
	s.registerGraphQuery()
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

// parseScopeString is a package-level alias for scopeutil.ParseScopeString.
var parseScopeString = scopeutil.ParseScopeString
