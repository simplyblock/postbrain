package rest

import (
	"net/http"

	"github.com/go-chi/chi/v5"
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

type updateScopeOwnerRequest struct {
	PrincipalID uuid.UUID `json:"principal_id"`
}

func (ro *Router) listScopes(w http.ResponseWriter, r *http.Request) {
	pg := paginationFromRequest(r)
	authorizedScopeIDs, err := ro.authorizedScopeIDsForRequest(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to resolve authorized scopes")
		return
	}
	allScopes, err := db.GetScopesByIDs(r.Context(), ro.pool, authorizedScopeIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pg.Offset >= len(allScopes) {
		writeJSON(w, http.StatusOK, map[string]any{"scopes": []*db.Scope{}})
		return
	}
	end := pg.Offset + pg.Limit
	if end > len(allScopes) {
		end = len(allScopes)
	}
	writeJSON(w, http.StatusOK, map[string]any{"scopes": allScopes[pg.Offset:end]})
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
	callerID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	if body.ParentID != nil {
		if err := ro.authorizeScopeAdmin(r.Context(), *body.ParentID); err != nil {
			writeScopeAuthzError(w, r, *body.ParentID, err)
			return
		}
	} else if callerID != body.PrincipalID {
		writeError(w, http.StatusForbidden, "forbidden: scope admin required")
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
	if err := ro.authorizeScopeAdmin(r.Context(), id); err != nil {
		writeScopeAuthzError(w, r, id, err)
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
	if err := ro.authorizeScopeAdmin(r.Context(), id); err != nil {
		writeScopeAuthzError(w, r, id, err)
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

// updateScopeOwner handles PUT /v1/scopes/{id}/owner.
func (ro *Router) updateScopeOwner(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scope id")
		return
	}
	var body updateScopeOwnerRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.PrincipalID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "principal_id is required")
		return
	}
	if err := ro.authorizeScopeAdmin(r.Context(), id); err != nil {
		writeScopeAuthzError(w, r, id, err)
		return
	}
	s, err := db.UpdateScopeOwner(r.Context(), ro.pool, id, body.PrincipalID)
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
	if err := ro.authorizeScopeAdmin(r.Context(), id); err != nil {
		writeScopeAuthzError(w, r, id, err)
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
	// SSHKey is a PEM-encoded private key for SSH clone URLs.
	SSHKey           string `json:"ssh_key,omitempty"`
	SSHKeyPassphrase string `json:"ssh_key_passphrase,omitempty"`
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
	if err := ro.authorizeScopeAdmin(r.Context(), id); err != nil {
		writeScopeAuthzError(w, r, id, err)
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
		ScopeID:          scope.ID,
		AuthorID:         principalID,
		RepoURL:          *scope.RepoUrl,
		DefaultBranch:    scope.RepoDefaultBranch,
		AuthToken:        body.AuthToken,
		SSHKey:           body.SSHKey,
		SSHKeyPassphrase: body.SSHKeyPassphrase,
		PrevCommit:       prevCommit,
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

func (ro *Router) registerScopeRoutes(r chi.Router) {
	r.Get("/scopes", ro.listScopes)
	r.Post("/scopes", ro.createScope)
	r.Get("/scopes/{id}", ro.getScope)
	r.Put("/scopes/{id}", ro.updateScope)
	r.Put("/scopes/{id}/owner", ro.updateScopeOwner)
	r.Delete("/scopes/{id}", ro.deleteScope)
	r.Post("/scopes/{id}/repo", ro.setScopeRepo)
	r.Post("/scopes/{id}/repo/sync", ro.syncScopeRepo)
	r.Get("/scopes/{id}/repo/sync", ro.getSyncStatus)
	r.Post("/scopes/{id}/grants", ro.handleCreateScopeGrant)
	r.Get("/scopes/{id}/grants", ro.handleListScopeGrants)
	r.Delete("/scopes/{id}/grants/{grant_id}", ro.handleDeleteScopeGrant)
}
