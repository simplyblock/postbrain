// Package rest provides the HTTP/JSON REST API for Postbrain.
package rest

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/codegraph"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/graph"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/memory"
	"github.com/simplyblock/postbrain/internal/principals"
	"github.com/simplyblock/postbrain/internal/providers"
	"github.com/simplyblock/postbrain/internal/sharing"
	"github.com/simplyblock/postbrain/internal/skills"
)

// Router holds all dependencies and builds the chi HTTP handler.
type Router struct {
	pool         *pgxpool.Pool
	svc          *providers.EmbeddingService
	cfg          *config.Config
	memStore     *memory.Store
	knwStore     *knowledge.Store
	sklStore     *skills.Store
	knwLife      *knowledge.Lifecycle
	sklLife      *skills.Lifecycle
	knwColl      *knowledge.CollectionStore
	knwProm      *knowledge.Promoter
	membership   *principals.MembershipStore
	principals   *principals.Store
	sharing      *sharing.Store
	graphStore   *graph.Store
	consolidator *memory.Consolidator
	syncer       *codegraph.Syncer
}

// Handler builds and returns the chi HTTP handler with all routes registered.
func (ro *Router) Handler() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health endpoint (unauthenticated).
	r.Get("/health", ro.handleHealth)

	// All /v1 routes require bearer token authentication.
	var authMW func(http.Handler) http.Handler
	if ro.pool != nil {
		authMW = auth.BearerTokenMiddleware(auth.NewTokenStore(ro.pool), ro.pool)
	} else {
		// In tests with nil pool we still enforce auth by rejecting all requests.
		authMW = func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				writeError(w, http.StatusUnauthorized, "unauthorized")
			})
		}
	}

	r.Route("/v1", func(r chi.Router) {
		r.Use(authMW)
		r.Use(ro.permissionAuthzMiddleware)
		r.Use(requestLoggerMiddleware)
		r.Use(ro.scopeAuthzContextMiddleware)

		ro.registerMemoryRoutes(r)
		ro.registerKnowledgeRoutes(r)
		ro.registerCollectionRoutes(r)
		ro.registerSkillRoutes(r)
		ro.registerSharingRoutes(r)
		ro.registerPromotionRoutes(r)
		ro.registerScopeRoutes(r)
		ro.registerPrincipalRoutes(r)
		ro.registerSessionRoutes(r)
		ro.registerContextRoutes(r)
		ro.registerGraphRoutes(r)
	})

	return r
}
