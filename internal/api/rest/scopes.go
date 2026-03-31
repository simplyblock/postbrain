package rest

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/codegraph"
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
	children, err := db.CountChildScopes(r.Context(), ro.pool, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if children > 0 {
		writeError(w, http.StatusConflict, "cannot delete scope: it has child scopes that must be deleted first")
		return
	}
	if err := db.DeleteScope(r.Context(), ro.pool, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type setScopeRepoRequest struct {
	RepoURL       string `json:"repo_url"`
	DefaultBranch string `json:"default_branch"`
}

// setScopeRepo handles POST /v1/scopes/{id}/repo.
// Attaches a git repository to a project-kind scope.
func (ro *Router) setScopeRepo(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scope id")
		return
	}
	var body setScopeRepoRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.RepoURL == "" {
		writeError(w, http.StatusBadRequest, "repo_url is required")
		return
	}
	if body.DefaultBranch == "" {
		body.DefaultBranch = "main"
	}
	s, err := db.SetScopeRepo(r.Context(), ro.pool, id, body.RepoURL, body.DefaultBranch)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s)
}

type syncRepoRequest struct {
	// AuthToken overrides the stored token for this single sync request.
	AuthToken string `json:"auth_token,omitempty"`
}

// syncScopeRepo handles POST /v1/scopes/{id}/repo/sync.
// Enqueues a background index run and returns 202 immediately.
func (ro *Router) syncScopeRepo(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scope id")
		return
	}

	scope, err := db.GetScopeByID(r.Context(), ro.pool, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if scope == nil {
		writeError(w, http.StatusNotFound, "scope not found")
		return
	}
	if scope.RepoUrl == nil || *scope.RepoUrl == "" {
		writeError(w, http.StatusBadRequest, "no repository attached to this scope")
		return
	}

	var body syncRepoRequest
	_ = readJSON(r, &body) // optional body

	prevCommit := ""
	if scope.LastIndexedCommit != nil {
		prevCommit = *scope.LastIndexedCommit
	}

	principalID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	opts := codegraph.IndexOptions{
		ScopeID:       scope.ID,
		AuthorID:      principalID,
		RepoURL:       *scope.RepoUrl,
		DefaultBranch: scope.RepoDefaultBranch,
		AuthToken:     body.AuthToken,
		PrevCommit:    prevCommit,
	}

	started, status := ro.syncer.Start(ro.pool, opts)
	if !started {
		writeJSON(w, http.StatusConflict, status)
		return
	}
	writeJSON(w, http.StatusAccepted, status)
}

// getSyncStatus handles GET /v1/scopes/{id}/repo/sync.
func (ro *Router) getSyncStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scope id")
		return
	}
	writeJSON(w, http.StatusOK, ro.syncer.Status(id))
}
