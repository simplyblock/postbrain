package rest

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/codegraph"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/providers"
	"github.com/simplyblock/postbrain/internal/graph"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/memory"
	"github.com/simplyblock/postbrain/internal/principals"
	"github.com/simplyblock/postbrain/internal/sharing"
	"github.com/simplyblock/postbrain/internal/skills"
)

// NewRouter creates a Router with all stores initialised.
func NewRouter(pool *pgxpool.Pool, svc *providers.EmbeddingService, cfg *config.Config) *Router {
	r := &Router{
		pool:   pool,
		svc:    svc,
		cfg:    cfg,
		syncer: codegraph.NewSyncer(),
	}
	if pool != nil {
		r.memStore = memory.NewStore(pool, svc)
		r.knwStore = knowledge.NewStore(pool, svc)
		r.sklStore = skills.NewStore(pool, svc)
		r.membership = principals.NewMembershipStore(pool)
		r.knwLife = knowledge.NewLifecycle(pool, r.membership)
		r.sklLife = skills.NewLifecycle(pool, r.membership)
		r.knwColl = knowledge.NewCollectionStore(pool)
		r.knwProm = knowledge.NewPromoter(pool, svc)
		r.principals = principals.NewStore(pool)
		r.sharing = sharing.NewStore(pool)
		r.graphStore = graph.NewStore(pool)
		r.consolidator = memory.NewConsolidator(pool, svc)
	}
	return r
}
