package rest

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
)

type createCollectionRequest struct {
	Scope       string  `json:"scope"`
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	Visibility  string  `json:"visibility"`
	Description *string `json:"description"`
}

func (r *createCollectionRequest) validate() error {
	if r.Scope == "" || r.Slug == "" || r.Name == "" {
		return errors.New("scope, slug and name are required")
	}
	return nil
}

func (r *createCollectionRequest) applyDefaults() {
	if r.Visibility == "" {
		r.Visibility = "team"
	}
}

func (ro *Router) createCollection(w http.ResponseWriter, r *http.Request) {
	var body createCollectionRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := body.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	body.applyDefaults()
	kind, externalID, err := parseScopeString(body.Scope)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	scope, err := db.GetScopeByExternalID(r.Context(), ro.pool, kind, externalID)
	if err != nil || scope == nil {
		writeError(w, http.StatusBadRequest, "scope not found")
		return
	}
	if err := ro.authorizeRequestedScope(r.Context(), scope.ID); err != nil {
		writeScopeAuthzError(w, r, scope.ID, err)
		return
	}

	ownerID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	coll, err := ro.knwColl.Create(r.Context(), scope.ID, ownerID, body.Slug, body.Name, body.Visibility, body.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, coll)
}

func (ro *Router) listCollections(w http.ResponseWriter, r *http.Request) {
	scopeStr := r.URL.Query().Get("scope")
	var scopeID uuid.UUID
	if scopeStr != "" {
		kind, externalID, err := parseScopeString(scopeStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		scope, err := db.GetScopeByExternalID(r.Context(), ro.pool, kind, externalID)
		if err != nil || scope == nil {
			writeError(w, http.StatusBadRequest, "scope not found")
			return
		}
		if err := ro.authorizeRequestedScope(r.Context(), scope.ID); err != nil {
			writeScopeAuthzError(w, r, scope.ID, err)
			return
		}
		scopeID = scope.ID
	}

	colls, err := ro.knwColl.List(r.Context(), scopeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"collections": colls})
}

func (ro *Router) getCollection(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	// Try to parse as UUID first, otherwise treat as slug.
	id, err := uuidParam(r, "slug")
	if err == nil {
		coll, err := ro.knwColl.GetByID(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if coll == nil {
			writeError(w, http.StatusNotFound, "collection not found")
			return
		}
		if err := ro.authorizeObjectScope(r.Context(), coll.ScopeID); err != nil {
			writeScopeAuthzError(w, r, coll.ScopeID, err)
			return
		}
		writeJSON(w, http.StatusOK, coll)
		return
	}
	// Look up by slug — requires ?scope=kind:external_id query param.
	scopeStr := r.URL.Query().Get("scope")
	if scopeStr == "" {
		writeError(w, http.StatusBadRequest, "provide collection UUID as path param or add ?scope=kind:external_id for slug lookup")
		return
	}
	kind, externalID, err := parseScopeString(scopeStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	scope, err := db.GetScopeByExternalID(r.Context(), ro.pool, kind, externalID)
	if err != nil || scope == nil {
		writeError(w, http.StatusBadRequest, "scope not found")
		return
	}
	if err := ro.authorizeRequestedScope(r.Context(), scope.ID); err != nil {
		writeScopeAuthzError(w, r, scope.ID, err)
		return
	}
	coll, err := ro.knwColl.GetBySlug(r.Context(), scope.ID, slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if coll == nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	writeJSON(w, http.StatusOK, coll)
}

type addCollectionItemRequest struct {
	ArtifactID string `json:"artifact_id"`
}

func (ro *Router) addCollectionItem(w http.ResponseWriter, r *http.Request) {
	collID, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid collection id")
		return
	}
	var body addCollectionItemRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	artID, err := uuid.Parse(body.ArtifactID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid artifact_id")
		return
	}
	coll, err := ro.knwColl.GetByID(r.Context(), collID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if coll == nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	if err := ro.authorizeDeleteObjectScope(r.Context(), coll.ScopeID); err != nil {
		writeScopeAuthzError(w, r, coll.ScopeID, err)
		return
	}
	adderID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	if err := ro.knwColl.AddItem(r.Context(), collID, artID, adderID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"collection_id": collID, "artifact_id": artID})
}

func (ro *Router) removeCollectionItem(w http.ResponseWriter, r *http.Request) {
	collID, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid collection id")
		return
	}
	artID, err := uuidParam(r, "artifact_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid artifact_id")
		return
	}
	coll, err := ro.knwColl.GetByID(r.Context(), collID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if coll == nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	if err := ro.authorizeObjectScope(r.Context(), coll.ScopeID); err != nil {
		writeScopeAuthzError(w, r, coll.ScopeID, err)
		return
	}
	if err := ro.knwColl.RemoveItem(r.Context(), collID, artID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
