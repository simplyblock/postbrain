package rest

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
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

	pg := paginationFromRequest(r)

	entities, err := db.ListEntitiesByScope(r.Context(), ro.pool, scopeID, "", pg.Limit, pg.Offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list entities")
		return
	}
	relations, err := db.ListRelationsByScope(r.Context(), ro.pool, scopeID, pg.Limit, pg.Offset)
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
// Returns {"error": "AGE unavailable"} with 501 (AGE not yet implemented).
func (ro *Router) queryCypher(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "AGE unavailable")
}
