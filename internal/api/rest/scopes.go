package rest

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
)

type createScopeRequest struct {
	Kind        string     `json:"kind"`
	ExternalID  string     `json:"external_id"`
	Name        string     `json:"name"`
	PrincipalID uuid.UUID  `json:"principal_id"`
	ParentID    *uuid.UUID `json:"parent_id,omitempty"`
	Meta        []byte     `json:"meta,omitempty"`
}

type updateScopeRequest struct {
	Name string `json:"name"`
	Meta []byte `json:"meta,omitempty"`
}

func (ro *Router) listScopes(w http.ResponseWriter, r *http.Request) {
	pg := paginationFromRequest(r)
	scopes, err := db.ListScopes(r.Context(), ro.pool, pg.Limit, pg.Offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scopes": scopes})
}

func (ro *Router) createScope(w http.ResponseWriter, r *http.Request) {
	var body createScopeRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Kind == "" || body.ExternalID == "" || body.Name == "" || body.PrincipalID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "kind, external_id, name and principal_id are required")
		return
	}
	s, err := db.CreateScope(r.Context(), ro.pool, body.Kind, body.ExternalID, body.Name, body.ParentID, body.PrincipalID, body.Meta)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

func (ro *Router) getScope(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scope id")
		return
	}
	s, err := db.GetScopeByID(r.Context(), ro.pool, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s == nil {
		writeError(w, http.StatusNotFound, "scope not found")
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (ro *Router) updateScope(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scope id")
		return
	}
	var body updateScopeRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	s, err := db.UpdateScope(r.Context(), ro.pool, id, body.Name, body.Meta)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s == nil {
		writeError(w, http.StatusNotFound, "scope not found")
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (ro *Router) deleteScope(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scope id")
		return
	}
	if err := db.DeleteScope(r.Context(), ro.pool, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
