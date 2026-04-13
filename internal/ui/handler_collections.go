package ui

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
)

// handleCollections serves GET /ui/collections.
func (h *Handler) handleCollections(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Collections []*db.KnowledgeCollection
	}{}

	if h.pool != nil {
		_, scopeSet := h.authorizedScopesForRequest(r.Context(), r)
		var colls []*db.KnowledgeCollection
		var err error
		scopeStr := r.URL.Query().Get("scope_id")
		if scopeStr != "" {
			sid, parseErr := uuid.Parse(scopeStr)
			if parseErr == nil {
				if _, ok := scopeSet[sid]; !ok {
					data.Collections = []*db.KnowledgeCollection{}
					h.render(w, r, "collections", "Collections", data)
					return
				}
				colls, err = compat.ListCollections(r.Context(), h.pool, sid)
			}
		} else {
			colls, err = compat.ListAllCollections(r.Context(), h.pool)
		}
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

// handleCollectionDetail serves GET /ui/collections/{id}.
func (h *Handler) handleCollectionDetail(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/ui/collections/")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if h.pool == nil {
		http.NotFound(w, r)
		return
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
	}{coll, arts})
}

// handleCollectionNew serves GET /ui/collections/new.
func (h *Handler) handleCollectionNew(w http.ResponseWriter, r *http.Request) {
	h.renderCollectionNew(w, r, "")
}

func (h *Handler) renderCollectionNew(w http.ResponseWriter, r *http.Request, formError string) {
	data := struct {
		FormError string
		Scopes    []*db.Scope
	}{FormError: formError}
	if h.pool != nil {
		scopes, _ := h.authorizedScopesForRequest(r.Context(), r)
		data.Scopes = scopes
	}
	h.render(w, r, "collections_new", "New Collection", data)
}

// handleCreateCollection serves POST /ui/collections.
func (h *Handler) handleCreateCollection(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
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
	scopeStr := strings.TrimSpace(r.FormValue("scope_id"))
	if scopeStr == "" {
		h.renderCollectionNew(w, r, "scope is required")
		return
	}
	scopeID, err := uuid.Parse(scopeStr)
	if err != nil {
		h.renderCollectionNew(w, r, "invalid scope id")
		return
	}
	_, authorizedScopeSet := h.authorizedScopesForRequest(r.Context(), r)
	if _, ok := authorizedScopeSet[scopeID]; !ok {
		h.renderCollectionNew(w, r, "scope access denied")
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
		ScopeID:    scopeID,
		OwnerID:    ownerID,
		Name:       name,
		Slug:       slug,
		Visibility: visibility,
	})
	if err != nil {
		h.renderCollectionNew(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/ui/collections/"+coll.ID.String(), http.StatusSeeOther)
}
