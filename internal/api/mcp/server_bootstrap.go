package mcp

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/providers"
	"github.com/simplyblock/postbrain/internal/graph"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/memory"
	"github.com/simplyblock/postbrain/internal/principals"
	"github.com/simplyblock/postbrain/internal/skills"
)

// NewServer creates all stores, registers all tools, and returns the Server.
func NewServer(pool *pgxpool.Pool, svc *providers.EmbeddingService, cfg *config.Config) *Server {
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
		s.ageEnabled = graph.DetectAGE(context.Background(), pool)
	}

	s.mcpServer = mcpserver.NewMCPServer("postbrain", "1.0.0")
	s.registerTools()
	s.registerResources()
	return s
}
