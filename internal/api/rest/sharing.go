package rest

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/sharing"
)

type createGrantRequest struct {
	MemoryID       *string `json:"memory_id"`
	ArtifactID     *string `json:"artifact_id"`
	GranteeScopeID string  `json:"grantee_scope_id"`
	CanReshare     bool    `json:"can_reshare"`
	ExpiresAt      *string `json:"expires_at"`
}

func (ro *Router) createGrant(w http.ResponseWriter, r *http.Request) {
	var body createGrantRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.GranteeScopeID == "" {
		writeError(w, http.StatusBadRequest, "grantee_scope_id is required")
		return
	}

	granteeScopeID, err := uuid.Parse(body.GranteeScopeID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid grantee_scope_id")
		return
	}

	grantedBy, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	g := &sharing.Grant{
		GranteeScopeID: granteeScopeID,
		GrantedBy:      grantedBy,
		CanReshare:     body.CanReshare,
	}
	if body.MemoryID != nil {
		id, err := uuid.Parse(*body.MemoryID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid memory_id")
			return
		}
		g.MemoryID = &id
	}
	if body.ArtifactID != nil {
		id, err := uuid.Parse(*body.ArtifactID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid artifact_id")
			return
		}
		g.ArtifactID = &id
	}
	if body.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *body.ExpiresAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid expires_at format, use RFC3339")
			return
		}
		g.ExpiresAt = &t
	}

	created, err := ro.sharing.Create(r.Context(), g)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (ro *Router) revokeGrant(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid grant id")
		return
	}
	if err := ro.sharing.Revoke(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (ro *Router) listGrants(w http.ResponseWriter, r *http.Request) {
	scopeStr := r.URL.Query().Get("grantee_scope_id")
	pg := paginationFromRequest(r)

	var granteeScopeID uuid.UUID
	if scopeStr != "" {
		id, err := uuid.Parse(scopeStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid grantee_scope_id")
			return
		}
		granteeScopeID = id
	}

	grants, err := ro.sharing.List(r.Context(), granteeScopeID, pg.Limit, pg.Offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"grants": grants})
}

func (ro *Router) registerSharingRoutes(r chi.Router) {
	r.Post("/sharing/grants", ro.createGrant)
	r.Delete("/sharing/grants/{id}", ro.revokeGrant)
	r.Get("/sharing/grants", ro.listGrants)
}
