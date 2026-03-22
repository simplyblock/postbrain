package rest

import (
	"net/http"

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

func (ro *Router) createCollection(w http.ResponseWriter, r *http.Request) {
	var body createCollectionRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Scope == "" || body.Slug == "" || body.Name == "" {
		writeError(w, http.StatusBadRequest, "scope, slug and name are required")
		return
	}
	visibility := body.Visibility
	if visibility == "" {
		visibility = "team"
	}

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

	ownerID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	coll, err := ro.knwColl.Create(r.Context(), scope.ID, ownerID, body.Slug, body.Name, visibility, body.Description)
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
	slug := r.PathValue("slug")
	if slug == "" {
		// chi stores it differently; fall back to URL param.
		slug = r.URL.Query().Get("slug")
	}
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
		writeJSON(w, http.StatusOK, coll)
		return
	}
	// Look up by slug (requires scope context).
	// TODO: require scope query param for slug lookups.
	writeError(w, http.StatusBadRequest, "provide collection UUID as path param")
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
	if err := ro.knwColl.RemoveItem(r.Context(), collID, artID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
