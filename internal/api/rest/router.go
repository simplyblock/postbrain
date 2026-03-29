// Package rest provides the HTTP/JSON REST API for Postbrain.
package rest

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/embedding"
	"github.com/simplyblock/postbrain/internal/graph"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/memory"
	"github.com/simplyblock/postbrain/internal/principals"
	"github.com/simplyblock/postbrain/internal/sharing"
	"github.com/simplyblock/postbrain/internal/skills"
)

// Router holds all dependencies and builds the chi HTTP handler.
type Router struct {
	pool         *pgxpool.Pool
	svc          *embedding.EmbeddingService
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
}

// NewRouter creates a Router with all stores initialised.
func NewRouter(pool *pgxpool.Pool, svc *embedding.EmbeddingService, cfg *config.Config) *Router {
	r := &Router{
		pool: pool,
		svc:  svc,
		cfg:  cfg,
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
		r.Use(requestLoggerMiddleware)

		// Memory endpoints.
		r.Post("/memories", ro.createMemory)
		r.Post("/memories/summarize", ro.handleSummarizeMemories)
		r.Get("/memories/recall", ro.recallMemories)
		r.Get("/memories/{id}", ro.getMemory)
		r.Patch("/memories/{id}", ro.updateMemory)
		r.Delete("/memories/{id}", ro.deleteMemory)
		r.Post("/memories/{id}/promote", ro.promoteMemory)

		// Knowledge endpoints.
		r.Post("/knowledge/upload", ro.uploadKnowledge)
		r.Post("/knowledge/synthesize", ro.synthesizeKnowledge)
		r.Post("/knowledge", ro.createArtifact)
		r.Get("/knowledge/search", ro.searchArtifacts)
		r.Get("/knowledge/{id}", ro.getArtifact)
		r.Patch("/knowledge/{id}", ro.updateArtifact)
		r.Delete("/knowledge/{id}", ro.deleteArtifact)
		r.Post("/knowledge/{id}/endorse", ro.endorseArtifact)
		r.Post("/knowledge/{id}/deprecate", ro.deprecateArtifact)
		r.Get("/knowledge/{id}/history", ro.getArtifactHistory)
		r.Get("/knowledge/{id}/sources", ro.getArtifactSources)
		r.Get("/knowledge/{id}/digests", ro.getArtifactDigests)

		// Collections.
		r.Post("/collections", ro.createCollection)
		r.Get("/collections", ro.listCollections)
		r.Get("/collections/{slug}", ro.getCollection)
		r.Post("/collections/{id}/items", ro.addCollectionItem)
		r.Delete("/collections/{id}/items/{artifact_id}", ro.removeCollectionItem)

		// Skills.
		r.Post("/skills", ro.createSkill)
		r.Get("/skills/search", ro.searchSkills)
		r.Get("/skills/{id}", ro.getSkill)
		r.Patch("/skills/{id}", ro.updateSkill)
		r.Post("/skills/{id}/endorse", ro.endorseSkill)
		r.Post("/skills/{id}/deprecate", ro.deprecateSkill)
		r.Post("/skills/{id}/install", ro.installSkill)
		r.Post("/skills/{id}/invoke", ro.invokeSkill)

		// Sharing grants.
		r.Post("/sharing/grants", ro.createGrant)
		r.Delete("/sharing/grants/{id}", ro.revokeGrant)
		r.Get("/sharing/grants", ro.listGrants)

		// Promotion requests.
		r.Get("/promotions", ro.listPromotions)
		r.Post("/promotions/{id}/approve", ro.approvePromotion)
		r.Post("/promotions/{id}/reject", ro.rejectPromotion)

		// Scopes & hierarchy.
		r.Get("/scopes", ro.listScopes)
		r.Post("/scopes", ro.createScope)
		r.Get("/scopes/{id}", ro.getScope)
		r.Put("/scopes/{id}", ro.updateScope)
		r.Delete("/scopes/{id}", ro.deleteScope)

		// Principals & membership.
		r.Get("/principals", ro.listPrincipals)
		r.Post("/principals", ro.createPrincipal)
		r.Get("/principals/{id}", ro.getPrincipal)
		r.Put("/principals/{id}", ro.updatePrincipal)
		r.Delete("/principals/{id}", ro.deletePrincipal)
		r.Get("/principals/{id}/members", ro.listMembers)
		r.Post("/principals/{id}/members", ro.addMember)
		r.Delete("/principals/{id}/members/{member_id}", ro.removeMember)

		// Sessions.
		r.Post("/sessions", ro.createSession)
		r.Patch("/sessions/{id}", ro.updateSession)

		// Context bundle.
		r.Get("/context", ro.getContext)

		// Graph endpoints.
		r.Get("/entities", ro.listEntities)
		r.Get("/graph", ro.getGraph)
		r.Post("/graph/query", ro.queryCypher)
	})

	return r
}
