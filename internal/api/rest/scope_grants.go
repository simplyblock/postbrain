package rest

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/authz"
	"github.com/simplyblock/postbrain/internal/db"
)

type createScopeGrantRequest struct {
	PrincipalID string   `json:"principal_id"`
	Permissions []string `json:"permissions"`
	ExpiresAt   *string  `json:"expires_at,omitempty"`
}

type scopeGrantResponse struct {
	ID          uuid.UUID  `json:"id"`
	PrincipalID uuid.UUID  `json:"principal_id"`
	ScopeID     uuid.UUID  `json:"scope_id"`
	Permissions []string   `json:"permissions"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

func scopeGrantToResponse(g *db.ScopeGrant) scopeGrantResponse {
	return scopeGrantResponse{
		ID:          g.ID,
		PrincipalID: g.PrincipalID,
		ScopeID:     g.ScopeID,
		Permissions: g.Permissions,
		ExpiresAt:   g.ExpiresAt,
		CreatedAt:   g.CreatedAt,
	}
}

// handleCreateScopeGrant serves POST /v1/scopes/{id}/grants.
// Requires sharing:write on the scope. Enforces anti-escalation: the caller
// cannot grant permissions they do not themselves hold on the scope.
func (ro *Router) handleCreateScopeGrant(w http.ResponseWriter, r *http.Request) {
	scopeID, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scope id")
		return
	}

	if err := ro.authorizeRequestedScope(r.Context(), scopeID); err != nil {
		writeScopeAuthzError(w, r, scopeID, err)
		return
	}

	var body createScopeGrantRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.PrincipalID == "" {
		writeError(w, http.StatusBadRequest, "principal_id is required")
		return
	}
	if len(body.Permissions) == 0 {
		writeError(w, http.StatusBadRequest, "permissions must not be empty")
		return
	}

	granteePrincipalID, err := uuid.Parse(body.PrincipalID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid principal_id")
		return
	}

	// Validate and parse the requested permissions.
	requestedPerms, err := authz.ParseTokenPermissions(body.Permissions)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid permissions: "+err.Error())
		return
	}

	// Anti-escalation: caller must hold every permission they are trying to grant.
	// System admins bypass this check because they hold all permissions by definition.
	// For all other callers we enforce token-level effective permissions (intersection of
	// the principal's permissions on this scope and the token's declared permissions).
	callerToken, _ := r.Context().Value(auth.ContextKeyToken).(*db.Token)
	tokenResolver, _ := r.Context().Value(auth.ContextKeyTokenResolver).(*authz.TokenResolver)
	if tokenResolver != nil && callerToken != nil {
		dbr := tokenResolver.DBResolver()
		isSystemAdmin := false
		if dbr != nil {
			principalPerms, pErr := dbr.EffectivePermissions(r.Context(), callerToken.PrincipalID, scopeID)
			if pErr == nil {
				// System admins receive all permissions; use that as the signal.
				isSystemAdmin = principalPerms.Len() == len(authz.AllPermissions())
			}
		}
		if !isSystemAdmin {
			callerEffective, err := tokenResolver.EffectiveTokenPermissions(r.Context(), callerToken, scopeID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to resolve caller permissions")
				return
			}
			for _, p := range requestedPerms.Permissions() {
				if !callerEffective.Contains(p) {
					writeError(w, http.StatusForbidden, "forbidden: cannot grant permission "+string(p)+" that caller does not hold")
					return
				}
			}
		}
	}

	var expiresAt *time.Time
	if body.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *body.ExpiresAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid expires_at format, use RFC3339")
			return
		}
		expiresAt = &t
	}

	callerPrincipalID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	var grantedBy *uuid.UUID
	if callerPrincipalID != uuid.Nil {
		grantedBy = &callerPrincipalID
	}

	rawPerms := make([]string, len(requestedPerms.Permissions()))
	for i, p := range requestedPerms.Permissions() {
		rawPerms[i] = string(p)
	}

	q := db.New(ro.pool)
	grant, err := q.CreateScopeGrant(r.Context(), db.CreateScopeGrantParams{
		PrincipalID: granteePrincipalID,
		ScopeID:     scopeID,
		Permissions: rawPerms,
		GrantedBy:   grantedBy,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, scopeGrantToResponse(grant))
}

// handleListScopeGrants serves GET /v1/scopes/{id}/grants.
// Requires sharing:read on the scope. Returns only non-expired grants.
func (ro *Router) handleListScopeGrants(w http.ResponseWriter, r *http.Request) {
	scopeID, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scope id")
		return
	}

	if err := ro.authorizeRequestedScope(r.Context(), scopeID); err != nil {
		writeScopeAuthzError(w, r, scopeID, err)
		return
	}

	q := db.New(ro.pool)
	grants, err := q.ListScopeGrantsByScope(r.Context(), scopeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Filter out expired grants.
	now := time.Now()
	var active []scopeGrantResponse
	for _, g := range grants {
		if g.ExpiresAt != nil && g.ExpiresAt.Before(now) {
			continue
		}
		active = append(active, scopeGrantToResponse(g))
	}
	if active == nil {
		active = []scopeGrantResponse{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"grants": active})
}

// handleDeleteScopeGrant serves DELETE /v1/scopes/{id}/grants/{grant_id}.
// Requires sharing:delete on the scope.
func (ro *Router) handleDeleteScopeGrant(w http.ResponseWriter, r *http.Request) {
	scopeID, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scope id")
		return
	}
	grantID, err := uuid.Parse(chi.URLParam(r, "grant_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid grant id")
		return
	}

	if err := ro.authorizeRequestedScope(r.Context(), scopeID); err != nil {
		writeScopeAuthzError(w, r, scopeID, err)
		return
	}

	q := db.New(ro.pool)
	if err := q.DeleteScopeGrant(r.Context(), grantID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
