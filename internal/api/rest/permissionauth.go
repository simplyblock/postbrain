package rest

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/authz"
	"github.com/simplyblock/postbrain/internal/db"
)

// routePermission maps a (method, chi route pattern) pair to the required
// authz.Permission. The pattern must exactly match what chi reports via
// chi.RouteContext(r.Context()).RoutePattern().
type routePermission struct {
	method  string
	pattern string
	perm    authz.Permission
}

// routePermissions is the authoritative per-route permission table.
// Order is not significant; every registered /v1 route must appear here.
var routePermissions = []routePermission{
	// Memories
	{http.MethodPost, "/v1/memories", "memories:write"},
	{http.MethodPost, "/v1/memories/summarize", "memories:write"},
	{http.MethodGet, "/v1/memories/recall", "memories:read"},
	{http.MethodGet, "/v1/memories/{id}", "memories:read"},
	{http.MethodPatch, "/v1/memories/{id}", "memories:write"},
	{http.MethodDelete, "/v1/memories/{id}", "memories:delete"},
	{http.MethodPost, "/v1/memories/{id}/promote", "promotions:write"},

	// Knowledge
	{http.MethodPost, "/v1/knowledge", "knowledge:write"},
	{http.MethodPost, "/v1/knowledge/upload", "knowledge:write"},
	{http.MethodPost, "/v1/knowledge/synthesize", "knowledge:write"},
	{http.MethodGet, "/v1/knowledge/search", "knowledge:read"},
	{http.MethodGet, "/v1/knowledge/{id}", "knowledge:read"},
	{http.MethodPatch, "/v1/knowledge/{id}", "knowledge:write"},
	{http.MethodDelete, "/v1/knowledge/{id}", "knowledge:delete"},
	{http.MethodPost, "/v1/knowledge/{id}/endorse", "knowledge:write"},
	{http.MethodPost, "/v1/knowledge/{id}/deprecate", "knowledge:edit"},
	{http.MethodGet, "/v1/knowledge/{id}/history", "knowledge:read"},
	{http.MethodGet, "/v1/knowledge/{id}/sources", "knowledge:read"},
	{http.MethodGet, "/v1/knowledge/{id}/digests", "knowledge:read"},

	// Collections
	{http.MethodGet, "/v1/collections", "collections:read"},
	{http.MethodPost, "/v1/collections", "collections:write"},
	{http.MethodGet, "/v1/collections/{slug}", "collections:read"},
	{http.MethodPost, "/v1/collections/{id}/items", "collections:write"},
	{http.MethodDelete, "/v1/collections/{id}/items/{artifact_id}", "collections:write"},

	// Skills
	{http.MethodGet, "/v1/skills/search", "skills:read"},
	{http.MethodPost, "/v1/skills", "skills:write"},
	{http.MethodGet, "/v1/skills/{id}", "skills:read"},
	{http.MethodPatch, "/v1/skills/{id}", "skills:write"},
	{http.MethodDelete, "/v1/skills/{id}", "skills:delete"},
	{http.MethodPost, "/v1/skills/{id}/endorse", "skills:write"},
	{http.MethodPost, "/v1/skills/{id}/deprecate", "skills:edit"},
	{http.MethodPost, "/v1/skills/{id}/install", "skills:read"},
	{http.MethodPost, "/v1/skills/{id}/invoke", "skills:read"},

	// Sessions
	{http.MethodPost, "/v1/sessions", "sessions:write"},
	{http.MethodPatch, "/v1/sessions/{id}", "sessions:write"},

	// Graph
	{http.MethodGet, "/v1/entities", "graph:read"},
	{http.MethodGet, "/v1/graph", "graph:read"},
	{http.MethodPost, "/v1/graph/query", "graph:read"},
	{http.MethodGet, "/v1/graph/callers", "graph:read"},
	{http.MethodGet, "/v1/graph/callees", "graph:read"},
	{http.MethodGet, "/v1/graph/deps", "graph:read"},
	{http.MethodGet, "/v1/graph/dependents", "graph:read"},

	// Sharing grants
	{http.MethodGet, "/v1/sharing/grants", "sharing:read"},
	{http.MethodPost, "/v1/sharing/grants", "sharing:write"},
	{http.MethodDelete, "/v1/sharing/grants/{id}", "sharing:delete"},

	// Promotions
	{http.MethodGet, "/v1/promotions", "promotions:read"},
	{http.MethodPost, "/v1/promotions/{id}/approve", "promotions:edit"},
	{http.MethodPost, "/v1/promotions/{id}/reject", "promotions:edit"},

	// Scopes
	{http.MethodGet, "/v1/scopes", "scopes:read"},
	{http.MethodPost, "/v1/scopes", "scopes:write"},
	{http.MethodGet, "/v1/scopes/{id}", "scopes:read"},
	{http.MethodPut, "/v1/scopes/{id}", "scopes:edit"},
	{http.MethodPut, "/v1/scopes/{id}/owner", "scopes:edit"},
	{http.MethodDelete, "/v1/scopes/{id}", "scopes:delete"},
	{http.MethodPost, "/v1/scopes/{id}/repo", "scopes:edit"},
	{http.MethodPost, "/v1/scopes/{id}/repo/sync", "scopes:edit"},
	{http.MethodGet, "/v1/scopes/{id}/repo/sync", "scopes:read"},

	// Principals & membership
	{http.MethodGet, "/v1/principals", "principals:read"},
	{http.MethodPost, "/v1/principals", "principals:write"},
	{http.MethodGet, "/v1/principals/{id}", "principals:read"},
	{http.MethodPut, "/v1/principals/{id}", "principals:edit"},
	{http.MethodDelete, "/v1/principals/{id}", "principals:delete"},
	{http.MethodGet, "/v1/principals/{id}/members", "principals:read"},
	{http.MethodPost, "/v1/principals/{id}/members", "principals:edit"},
	{http.MethodDelete, "/v1/principals/{id}/members/{member_id}", "principals:edit"},

	// Context bundle (requires any read permission — use memories:read as baseline)
	{http.MethodGet, "/v1/context", "memories:read"},
}

// routePermTable is a compiled lookup: "METHOD /pattern" → required permission.
var routePermTable map[string]authz.Permission

func init() {
	routePermTable = make(map[string]authz.Permission, len(routePermissions))
	for _, rp := range routePermissions {
		key := rp.method + " " + rp.pattern
		routePermTable[key] = rp.perm
	}
}

func (ro *Router) permissionAuthzMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, _ := r.Context().Value(auth.ContextKeyToken).(*db.Token)
		if token == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		perms, _ := r.Context().Value(auth.ContextKeyPermissions).(authz.PermissionSet)

		// Look up the required permission for this route.
		pattern := routePattern(r)
		key := r.Method + " " + pattern
		required, found := routePermTable[key]
		if !found {
			// No entry in the table: allow (health, unknown routes handled by chi).
			next.ServeHTTP(w, r)
			return
		}

		if !perms.Contains(required) {
			writeError(w, http.StatusForbidden, "forbidden: insufficient permissions")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// routePattern returns the chi route pattern for the current request, or the
// raw URL path if no pattern is set.
func routePattern(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil {
		if p := rc.RoutePattern(); p != "" {
			return p
		}
	}
	return r.URL.Path
}
