package rest

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/graph"
)

// listEntities handles GET /v1/entities?scope_id=<uuid>&type=<string>&limit=N&offset=N.
// Returns {"entities": [...], "total": N}.
func (ro *Router) listEntities(w http.ResponseWriter, r *http.Request) {
	scopeStr := r.URL.Query().Get("scope_id")
	if scopeStr == "" {
		writeError(w, http.StatusBadRequest, "scope_id is required")
		return
	}
	scopeID, err := uuid.Parse(scopeStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scope_id")
		return
	}
	if ro.pool == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	if err := ro.authorizeRequestedScope(r.Context(), scopeID); err != nil {
		writeScopeAuthzError(w, r, scopeID, err)
		return
	}

	entityType := r.URL.Query().Get("type")
	pg := paginationFromRequest(r)

	entities, err := db.ListEntitiesByScope(r.Context(), ro.pool, scopeID, entityType, pg.Limit, pg.Offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list entities")
		return
	}
	if entities == nil {
		entities = []*db.Entity{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"entities": entities,
		"total":    len(entities),
	})
}

// getGraph handles GET /v1/graph?scope_id=<uuid>.
// Returns the full entity+relation graph for a scope.
func (ro *Router) getGraph(w http.ResponseWriter, r *http.Request) {
	scopeStr := r.URL.Query().Get("scope_id")
	if scopeStr == "" {
		writeError(w, http.StatusBadRequest, "scope_id is required")
		return
	}
	scopeID, err := uuid.Parse(scopeStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scope_id")
		return
	}
	if ro.pool == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	if err := ro.authorizeRequestedScope(r.Context(), scopeID); err != nil {
		writeScopeAuthzError(w, r, scopeID, err)
		return
	}

	pg := paginationFromRequest(r)

	entities, err := db.ListEntitiesByScope(r.Context(), ro.pool, scopeID, "", pg.Limit, pg.Offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list entities")
		return
	}
	relations, err := db.ListRelationsByScope(r.Context(), ro.pool, scopeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list relations")
		return
	}
	if entities == nil {
		entities = []*db.Entity{}
	}
	if relations == nil {
		relations = []*db.Relation{}
	}

	type relationView struct {
		ID         uuid.UUID `json:"id"`
		SubjectID  uuid.UUID `json:"subject_id"`
		Predicate  string    `json:"predicate"`
		ObjectID   uuid.UUID `json:"object_id"`
		Confidence float64   `json:"confidence"`
	}
	rels := make([]relationView, len(relations))
	for i, rel := range relations {
		rels[i] = relationView{
			ID:         rel.ID,
			SubjectID:  rel.SubjectID,
			Predicate:  rel.Predicate,
			ObjectID:   rel.ObjectID,
			Confidence: rel.Confidence,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"entities":  entities,
		"relations": rels,
	})
}

// queryCypher handles POST /v1/graph/query.
// Body: {"cypher": "...", "scope_id": "..."}.
func (ro *Router) queryCypher(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cypher  string `json:"cypher"`
		ScopeID string `json:"scope_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Cypher) == "" {
		writeError(w, http.StatusBadRequest, "cypher is required")
		return
	}
	scopeID, err := uuid.Parse(req.ScopeID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scope_id")
		return
	}
	if ro.pool == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	if err := ro.authorizeRequestedScope(r.Context(), scopeID); err != nil {
		writeScopeAuthzError(w, r, scopeID, err)
		return
	}

	rows, err := graph.RunCypherQuery(r.Context(), ro.pool, scopeID, req.Cypher)
	if err != nil {
		if errors.Is(err, graph.ErrAGEUnavailable) {
			writeError(w, http.StatusNotImplemented, "AGE unavailable")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to run graph query")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows})
}

type traversalNeighbourJSON struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	Canonical  string    `json:"canonical"`
	Type       string    `json:"type"`
	Predicate  string    `json:"predicate"`
	Direction  string    `json:"direction"`
	Confidence float64   `json:"confidence"`
	SourceFile *string   `json:"source_file,omitempty"`
}

type traversalResultJSON struct {
	ID         uuid.UUID                `json:"id"`
	Name       string                   `json:"name"`
	Canonical  string                   `json:"canonical"`
	Type       string                   `json:"type"`
	Neighbours []traversalNeighbourJSON `json:"neighbours"`
}

func traversalResult(res *graph.TraversalResult) traversalResultJSON {
	out := traversalResultJSON{
		ID:         res.Entity.ID,
		Name:       res.Entity.Name,
		Canonical:  res.Entity.Canonical,
		Type:       res.Entity.EntityType,
		Neighbours: make([]traversalNeighbourJSON, 0, len(res.Neighbours)),
	}
	for _, n := range res.Neighbours {
		out.Neighbours = append(out.Neighbours, traversalNeighbourJSON{
			ID:         n.Entity.ID,
			Name:       n.Entity.Name,
			Canonical:  n.Entity.Canonical,
			Type:       n.Entity.EntityType,
			Predicate:  n.Predicate,
			Direction:  n.Direction,
			Confidence: n.Confidence,
			SourceFile: n.SourceFile,
		})
	}
	return out
}

func scopeAndSymbol(r *http.Request) (uuid.UUID, string, bool) {
	scopeStr := r.URL.Query().Get("scope_id")
	symbol := r.URL.Query().Get("symbol")
	if scopeStr == "" || symbol == "" {
		return uuid.UUID{}, "", false
	}
	id, err := uuid.Parse(scopeStr)
	if err != nil {
		return uuid.UUID{}, "", false
	}
	return id, symbol, true
}

// getCallers handles GET /v1/graph/callers?scope_id=&symbol=
func (ro *Router) getCallers(w http.ResponseWriter, r *http.Request) {
	scopeID, symbol, ok := scopeAndSymbol(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "scope_id and symbol are required")
		return
	}
	res, err := graph.Callers(r.Context(), ro.pool, scopeID, symbol)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if res == nil {
		writeError(w, http.StatusNotFound, "symbol not found")
		return
	}
	writeJSON(w, http.StatusOK, traversalResult(res))
}

// getCallees handles GET /v1/graph/callees?scope_id=&symbol=
func (ro *Router) getCallees(w http.ResponseWriter, r *http.Request) {
	scopeID, symbol, ok := scopeAndSymbol(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "scope_id and symbol are required")
		return
	}
	res, err := graph.Callees(r.Context(), ro.pool, scopeID, symbol)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if res == nil {
		writeError(w, http.StatusNotFound, "symbol not found")
		return
	}
	writeJSON(w, http.StatusOK, traversalResult(res))
}

// getDeps handles GET /v1/graph/deps?scope_id=&symbol=
func (ro *Router) getDeps(w http.ResponseWriter, r *http.Request) {
	scopeID, symbol, ok := scopeAndSymbol(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "scope_id and symbol are required")
		return
	}
	res, err := graph.Dependencies(r.Context(), ro.pool, scopeID, symbol)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if res == nil {
		writeError(w, http.StatusNotFound, "symbol not found")
		return
	}
	writeJSON(w, http.StatusOK, traversalResult(res))
}

// getDependents handles GET /v1/graph/dependents?scope_id=&symbol=
func (ro *Router) getDependents(w http.ResponseWriter, r *http.Request) {
	scopeID, symbol, ok := scopeAndSymbol(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "scope_id and symbol are required")
		return
	}
	res, err := graph.Dependents(r.Context(), ro.pool, scopeID, symbol)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if res == nil {
		writeError(w, http.StatusNotFound, "symbol not found")
		return
	}
	writeJSON(w, http.StatusOK, traversalResult(res))
}

func (ro *Router) registerGraphRoutes(r chi.Router) {
	r.Get("/entities", ro.listEntities)
	r.Get("/graph", ro.getGraph)
	r.Post("/graph/query", ro.queryCypher)
	r.Get("/graph/callers", ro.getCallers)
	r.Get("/graph/callees", ro.getCallees)
	r.Get("/graph/deps", ro.getDeps)
	r.Get("/graph/dependents", ro.getDependents)
}
