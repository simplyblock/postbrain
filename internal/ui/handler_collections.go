package ui

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
)

// handleCollections serves GET /ui/{scope}/collections.
func (h *Handler) handleCollections(w http.ResponseWriter, r *http.Request) {
	scope := scopeFromContext(r.Context())

	data := struct {
		ScopeID     string
		Collections []*db.KnowledgeCollection
	}{}
	if scope != nil {
		data.ScopeID = scope.ID.String()
	}

	if h.pool != nil && scope != nil {
		_, scopeSet := h.authorizedScopesForRequest(r.Context(), r)
		colls, err := compat.ListCollections(r.Context(), h.pool, scope.ID)
		if err != nil {
			http.Error(w, "failed to load collections", http.StatusInternalServerError)
			return
		}
		filtered := make([]*db.KnowledgeCollection, 0, len(colls))
		for _, c := range colls {
			if _, ok := scopeSet[c.ScopeID]; ok {
				filtered = append(filtered, c)
			}
		}
		data.Collections = filtered
	}

	h.render(w, r, "collections", "Collections", data)
}

// handleCollectionDetail serves GET /ui/{scope}/collections/{id}.
func (h *Handler) handleCollectionDetail(w http.ResponseWriter, r *http.Request) {
	path := routePathFromContext(r.Context(), r)
	idStr := strings.TrimPrefix(path, "/ui/collections/")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if h.pool == nil {
		http.NotFound(w, r)
		return
	}

	scopeID := ""
	if scope := scopeFromContext(r.Context()); scope != nil {
		scopeID = scope.ID.String()
	}

	coll, err := compat.GetCollection(r.Context(), h.pool, id)
	if err != nil || coll == nil {
		http.NotFound(w, r)
		return
	}
	arts, err := compat.ListCollectionItems(r.Context(), h.pool, id)
	if err != nil {
		http.Error(w, "failed to load collection items", http.StatusInternalServerError)
		return
	}
	h.render(w, r, "collection_detail", "Collection", struct {
		Collection *db.KnowledgeCollection
		Artifacts  []*db.KnowledgeArtifact
		ScopeID    string
	}{coll, arts, scopeID})
}

// handleCollectionNew serves GET /ui/{scope}/collections/new.
func (h *Handler) handleCollectionNew(w http.ResponseWriter, r *http.Request) {
	h.renderCollectionNew(w, r, "")
}

func (h *Handler) renderCollectionNew(w http.ResponseWriter, r *http.Request, formError string) {
	scopeID := ""
	if scope := scopeFromContext(r.Context()); scope != nil {
		scopeID = scope.ID.String()
	}
	data := struct {
		FormError string
		ScopeID   string
	}{FormError: formError, ScopeID: scopeID}
	h.render(w, r, "collections_new", "New Collection", data)
}

// handleCreateCollection serves POST /ui/{scope}/collections.
func (h *Handler) handleCreateCollection(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	scope := scopeFromContext(r.Context())
	if scope == nil {
		h.renderCollectionNew(w, r, "scope not found")
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		h.renderCollectionNew(w, r, "name is required")
		return
	}
	slug := strings.TrimSpace(r.FormValue("slug"))
	if slug == "" {
		h.renderCollectionNew(w, r, "slug is required")
		return
	}
	if h.pool == nil {
		h.renderCollectionNew(w, r, "service unavailable")
		return
	}
	visibility := r.FormValue("visibility")
	if visibility == "" {
		visibility = "team"
	}
	ownerID := h.principalFromCookie(r)
	coll, err := compat.CreateCollection(r.Context(), h.pool, &db.KnowledgeCollection{
		ScopeID:    scope.ID,
		OwnerID:    ownerID,
		Name:       name,
		Slug:       slug,
		Visibility: visibility,
	})
	if err != nil {
		h.renderCollectionNew(w, r, err.Error())
		return
	}
	http.Redirect(w, r, scopedPath(r, "/collections/"+coll.ID.String()), http.StatusSeeOther)
}
