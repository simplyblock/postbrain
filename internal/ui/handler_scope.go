package ui

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
)

type ctxKey int

const (
	ctxKeyCurrentScope ctxKey = iota
	ctxKeyAllScopes
	ctxKeyResourcePath
)

// parseScopedPath checks if path matches /ui/{uuid}/rest.
// Returns the UUID and the rest of the path (e.g. "/memories").
func parseScopedPath(path string) (scopeID uuid.UUID, rest string, ok bool) {
	const prefix = "/ui/"
	if !strings.HasPrefix(path, prefix) {
		return uuid.Nil, "", false
	}
	after := path[len(prefix):]
	idx := strings.Index(after, "/")
	var seg, remainder string
	if idx >= 0 {
		seg = after[:idx]
		remainder = after[idx:]
	} else {
		seg = after
		remainder = ""
	}
	id, err := uuid.Parse(seg)
	if err != nil {
		return uuid.Nil, "", false
	}
	return id, remainder, true
}

// scopeFromContext returns the current scope from the request context.
func scopeFromContext(ctx context.Context) *db.Scope {
	s, _ := ctx.Value(ctxKeyCurrentScope).(*db.Scope)
	return s
}

// scopesFromContext returns all authorized scopes from the request context.
func scopesFromContext(ctx context.Context) []*db.Scope {
	s, _ := ctx.Value(ctxKeyAllScopes).([]*db.Scope)
	return s
}

// routePathFromContext returns the logical /ui/... path with the scope prefix stripped.
// Handlers use this for ID extraction instead of r.URL.Path.
// e.g. /ui/{scope}/memories/{id} → /ui/memories/{id}
func routePathFromContext(ctx context.Context, r *http.Request) string {
	if p, ok := ctx.Value(ctxKeyResourcePath).(string); ok && p != "" {
		return p
	}
	return r.URL.Path
}

// scopedRedirect sends a SeeOther redirect to a page within the current scope.
// pagePath must start with "/", e.g. "/memories" or "/knowledge/abc".
func scopedRedirect(w http.ResponseWriter, r *http.Request, pagePath string) {
	http.Redirect(w, r, scopedPath(r, pagePath), http.StatusSeeOther)
}

// scopedPath builds the full URL for a page within the current scope.
func scopedPath(r *http.Request, pagePath string) string {
	if scope := scopeFromContext(r.Context()); scope != nil {
		return "/ui/" + scope.ID.String() + pagePath
	}
	return "/ui" + pagePath
}

// scopeRequiredPrefixes are /ui page roots that require a scope in the path.
var scopeRequiredPrefixes = []string{
	"/ui/memories",
	"/ui/query",
	"/ui/knowledge",
	"/ui/collections",
	"/ui/promotions",
	"/ui/staleness",
	"/ui/skills",
	"/ui/graph",
}

// dispatchScopedRoute handles /ui/{scope-uuid}/... requests.
// Validates the scope, enriches the request context, and dispatches to the
// appropriate handler. Returns true if the request was handled.
func (h *Handler) dispatchScopedRoute(w http.ResponseWriter, r *http.Request) bool {
	scopeID, rest, ok := parseScopedPath(r.URL.Path)
	if !ok {
		return false
	}

	// Resolve and validate the scope.
	scopes, scopeSet := h.authorizedScopesForRequest(r.Context(), r)
	if _, authorized := scopeSet[scopeID]; !authorized {
		http.Error(w, "forbidden: scope not authorized", http.StatusForbidden)
		return true
	}

	var currentScope *db.Scope
	for _, s := range scopes {
		if s.ID == scopeID {
			currentScope = s
			break
		}
	}

	// Enrich context with scope info so render() and handlers can access it.
	ctx := r.Context()
	ctx = context.WithValue(ctx, ctxKeyCurrentScope, currentScope)
	ctx = context.WithValue(ctx, ctxKeyAllScopes, scopes)
	ctx = context.WithValue(ctx, ctxKeyResourcePath, "/ui"+rest)
	r = r.WithContext(ctx)

	switch {
	case rest == "" || rest == "/":
		http.Redirect(w, r, "/ui/"+scopeID.String()+"/memories", http.StatusFound)

	// Memories
	case rest == "/memories":
		h.handleMemories(w, r)
	case strings.HasPrefix(rest, "/memories/") && strings.HasSuffix(rest, "/forget") && r.Method == http.MethodPost:
		h.handleMemoryForget(w, r)
	case strings.HasPrefix(rest, "/memories/"):
		h.handleMemoryDetail(w, r)

	// Query playground
	case rest == "/query":
		h.handleQuery(w, r)

	// Knowledge / artifacts
	case rest == "/knowledge/upload" && r.Method == http.MethodPost:
		h.handleUploadKnowledge(w, r)
	case rest == "/knowledge/new":
		h.handleKnowledgeNew(w, r)
	case rest == "/knowledge" && r.Method == http.MethodPost:
		h.handleCreateKnowledge(w, r)
	case rest == "/knowledge":
		h.handleKnowledge(w, r)
	case strings.HasPrefix(rest, "/knowledge/") && strings.HasSuffix(rest, "/endorse") && r.Method == http.MethodPost:
		h.handleEndorseArtifact(w, r)
	case strings.HasPrefix(rest, "/knowledge/") && strings.HasSuffix(rest, "/review") && r.Method == http.MethodPost:
		h.handleKnowledgeReview(w, r)
	case strings.HasPrefix(rest, "/knowledge/") && strings.HasSuffix(rest, "/retract") && r.Method == http.MethodPost:
		h.handleKnowledgeRetract(w, r)
	case strings.HasPrefix(rest, "/knowledge/") && strings.HasSuffix(rest, "/deprecate") && r.Method == http.MethodPost:
		h.handleKnowledgeDeprecate(w, r)
	case strings.HasPrefix(rest, "/knowledge/") && strings.HasSuffix(rest, "/republish") && r.Method == http.MethodPost:
		h.handleKnowledgeRepublish(w, r)
	case strings.HasPrefix(rest, "/knowledge/") && strings.HasSuffix(rest, "/delete") && r.Method == http.MethodPost:
		h.handleKnowledgeDelete(w, r)
	case strings.HasPrefix(rest, "/knowledge/") && strings.HasSuffix(rest, "/history"):
		h.handleKnowledgeHistory(w, r)
	case strings.HasPrefix(rest, "/knowledge/"):
		h.handleKnowledgeDetail(w, r)

	// Collections
	case rest == "/collections" && r.Method == http.MethodPost:
		h.handleCreateCollection(w, r)
	case rest == "/collections/new":
		h.handleCollectionNew(w, r)
	case rest == "/collections":
		h.handleCollections(w, r)
	case strings.HasPrefix(rest, "/collections/"):
		h.handleCollectionDetail(w, r)

	// Promotions
	case rest == "/promotions":
		h.handlePromotions(w, r)
	case strings.HasPrefix(rest, "/promotions/") && strings.HasSuffix(rest, "/approve") && r.Method == http.MethodPost:
		h.handleApprovePromotion(w, r)
	case strings.HasPrefix(rest, "/promotions/") && strings.HasSuffix(rest, "/reject") && r.Method == http.MethodPost:
		h.handleRejectPromotion(w, r)

	// Staleness
	case rest == "/staleness":
		h.handleStaleness(w, r)

	// Entity graphs
	case rest == "/graph":
		h.handleGraph(w, r)
	case rest == "/graph3d":
		h.handleGraph3D(w, r)

	// Skills
	case rest == "/skills":
		h.handleSkills(w, r)
	case strings.HasPrefix(rest, "/skills/") && strings.HasSuffix(rest, "/history"):
		h.handleSkillHistory(w, r)
	case strings.HasPrefix(rest, "/skills/"):
		h.handleSkillDetail(w, r)

	default:
		http.NotFound(w, r)
	}
	return true
}
