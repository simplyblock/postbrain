package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/authz"
	"github.com/simplyblock/postbrain/internal/codegraph"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/principals"
)

type scopeGrantRow struct {
	ID               uuid.UUID
	PrincipalID      uuid.UUID
	PrincipalSlug    string
	PrincipalName    string
	ScopeID          uuid.UUID
	ScopeDisplayName string
	Permissions      []string
	ExpiresAt        *time.Time
	CreatedAt        time.Time
	CanDelete        bool
}

type scopeGrantScopeOption struct {
	ID          uuid.UUID
	DisplayName string
}

type scopeGrantResourceOption struct {
	Resource   string
	Operations []string
}

// handlePrincipals serves GET /ui/principals.
func (h *Handler) handlePrincipals(w http.ResponseWriter, r *http.Request) {
	h.renderPrincipals(w, r, "", "", "", "")
}

// handleScopes serves GET /ui/scopes.
func (h *Handler) handleScopes(w http.ResponseWriter, r *http.Request) {
	h.renderScopes(w, r, "")
}

// renderScopes renders the scopes page with an optional form error.
func (h *Handler) renderScopes(w http.ResponseWriter, r *http.Request, scopeErr string) {
	data := struct {
		Principals     []*db.Principal
		Scopes         []*db.Scope
		ScopeFormError string
		SyncStatus     map[string]codegraph.SyncStatus
		ChildCount     map[string]int64
		CanManage      map[string]bool
		CanDelete      map[string]bool
		OwnerNames     map[string]string
	}{
		ScopeFormError: scopeErr,
		SyncStatus:     make(map[string]codegraph.SyncStatus),
		ChildCount:     make(map[string]int64),
		CanManage:      make(map[string]bool),
		CanDelete:      make(map[string]bool),
		OwnerNames:     make(map[string]string),
	}

	if h.pool != nil {
		scopes, writable := h.authorizedScopesForRequest(r.Context(), r)
		principals, err := db.ListPrincipals(r.Context(), h.pool, 50, 0)
		if err == nil {
			data.Principals = principals
			for _, p := range principals {
				data.OwnerNames[p.ID.String()] = p.DisplayName
			}
		}
		filtered := make([]*db.Scope, 0, len(scopes))
		for _, s := range scopes {
			if _, ok := writable[s.ID]; !ok {
				continue
			}
			filtered = append(filtered, s)
			st := h.syncer.Status(s.ID)
			if st.State != codegraph.SyncIdle || st.CommitSHA != "" || st.Error != "" {
				data.SyncStatus[s.ID.String()] = st
			}
			if n, err := db.CountChildScopes(r.Context(), h.pool, s.ID); err == nil && n > 0 {
				data.ChildCount[s.ID.String()] = n
			}
			canManage := h.hasScopeAdminAccess(r.Context(), r, s.ID)
			data.CanManage[s.ID.String()] = canManage
			data.CanDelete[s.ID.String()] = canManage
		}
		data.Scopes = filtered
	}

	h.render(w, r, "scopes", "Scopes", data)
}

// renderPrincipals renders the principals page with optional form errors.
func (h *Handler) renderPrincipals(
	w http.ResponseWriter,
	r *http.Request,
	principalErr, principalEditErr, membershipErr, scopeGrantErr string,
) {
	data := struct {
		Principals          []*db.Principal
		Memberships         []*db.MembershipRow
		PrincipalFormError  string
		PrincipalEditError  string
		MembershipFormError string
		ScopeGrantFormError string
		ScopeGrantRows      []scopeGrantRow
		ScopeGrantScopes    []scopeGrantScopeOption
		ScopeGrantResources []scopeGrantResourceOption
	}{
		PrincipalFormError:  principalErr,
		PrincipalEditError:  principalEditErr,
		MembershipFormError: membershipErr,
		ScopeGrantFormError: scopeGrantErr,
	}
	for _, resource := range authz.AllResources() {
		ops := authz.ValidOperations(resource)
		option := scopeGrantResourceOption{
			Resource:   string(resource),
			Operations: make([]string, 0, len(ops)),
		}
		for _, op := range ops {
			option.Operations = append(option.Operations, string(op))
		}
		data.ScopeGrantResources = append(data.ScopeGrantResources, option)
	}

	if h.pool != nil {
		reachable := h.reachablePrincipalIDSet(r.Context(), r)
		principals, err := db.ListPrincipals(r.Context(), h.pool, 50, 0)
		if err == nil {
			if reachable == nil {
				// nil means show all (system admin).
				data.Principals = principals
			} else {
				filtered := make([]*db.Principal, 0, len(principals))
				for _, p := range principals {
					if _, ok := reachable[p.ID]; !ok {
						continue
					}
					filtered = append(filtered, p)
				}
				data.Principals = filtered
			}
		}
		memberships, err := db.ListAllMemberships(r.Context(), h.pool)
		if err == nil {
			if reachable == nil {
				data.Memberships = memberships
			} else {
				filtered := make([]*db.MembershipRow, 0, len(memberships))
				for _, m := range memberships {
					if _, ok := reachable[m.MemberID]; !ok {
						continue
					}
					if _, ok := reachable[m.ParentID]; !ok {
						continue
					}
					filtered = append(filtered, m)
				}
				data.Memberships = filtered
			}
		}

		granteeNameByID := make(map[uuid.UUID]string, len(data.Principals))
		granteeSlugByID := make(map[uuid.UUID]string, len(data.Principals))
		for _, p := range data.Principals {
			granteeNameByID[p.ID] = p.DisplayName
			granteeSlugByID[p.ID] = p.Slug
		}

		scopes, _ := h.authorizedScopesForRequest(r.Context(), r)
		scopeDisplayByID := make(map[uuid.UUID]string, len(scopes))
		q := db.New(h.pool)
		for _, scope := range scopes {
			scopeDisplay := scope.Name + " (" + scope.ExternalID + ")"
			scopeDisplayByID[scope.ID] = scopeDisplay
			if h.hasScopePermission(r.Context(), r, scope.ID,
				authz.NewPermission(authz.ResourceSharing, authz.OperationWrite)) {
				data.ScopeGrantScopes = append(data.ScopeGrantScopes, scopeGrantScopeOption{
					ID:          scope.ID,
					DisplayName: scopeDisplay,
				})
			}
			if !h.hasScopePermission(r.Context(), r, scope.ID,
				authz.NewPermission(authz.ResourceSharing, authz.OperationRead)) {
				continue
			}
			grants, err := q.ListScopeGrantsByScope(r.Context(), scope.ID)
			if err != nil {
				continue
			}
			canDelete := h.hasScopePermission(r.Context(), r, scope.ID,
				authz.NewPermission(authz.ResourceSharing, authz.OperationDelete))
			for _, g := range grants {
				name := granteeNameByID[g.PrincipalID]
				if name == "" {
					name = g.PrincipalID.String()
				}
				slug := granteeSlugByID[g.PrincipalID]
				row := scopeGrantRow{
					ID:               g.ID,
					PrincipalID:      g.PrincipalID,
					PrincipalSlug:    slug,
					PrincipalName:    name,
					ScopeID:          g.ScopeID,
					ScopeDisplayName: scopeDisplayByID[g.ScopeID],
					Permissions:      slices.Clone(g.Permissions),
					ExpiresAt:        g.ExpiresAt,
					CreatedAt:        g.CreatedAt,
					CanDelete:        canDelete,
				}
				data.ScopeGrantRows = append(data.ScopeGrantRows, row)
			}
		}
		sort.Slice(data.ScopeGrantRows, func(i, j int) bool {
			return data.ScopeGrantRows[i].CreatedAt.After(data.ScopeGrantRows[j].CreatedAt)
		})
		sort.Slice(data.ScopeGrantScopes, func(i, j int) bool {
			return data.ScopeGrantScopes[i].DisplayName < data.ScopeGrantScopes[j].DisplayName
		})
	}

	h.render(w, r, "principals", "Principals", data)
}

// handleDeleteScope serves POST /ui/scopes/{id}/delete.
func (h *Handler) handleDeleteScope(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/ui/scopes/")
	idStr := strings.TrimSuffix(trimmed, "/delete")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.renderScopes(w, r, "invalid scope id")
		return
	}
	if h.pool == nil {
		h.renderScopes(w, r, "service unavailable")
		return
	}
	if !h.hasScopeAdminAccess(r.Context(), r, id) {
		h.renderScopes(w, r, "scope admin required")
		return
	}
	children, err := db.CountChildScopes(r.Context(), h.pool, id)
	if err != nil {
		h.renderScopes(w, r, "could not check for child scopes")
		return
	}
	if children > 0 {
		h.renderScopes(w, r, "cannot delete scope: it has child scopes that must be deleted first")
		return
	}
	if err := db.DeleteScope(r.Context(), h.pool, id); err != nil {
		h.renderScopes(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/ui/scopes", http.StatusSeeOther)
}

// handleSetScopeOwner serves POST /ui/scopes/{id}/owner.
func (h *Handler) handleSetScopeOwner(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/ui/scopes/")
	idStr := strings.TrimSuffix(trimmed, "/owner")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.renderScopes(w, r, "invalid scope id")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderScopes(w, r, "bad form data")
		return
	}
	principalIDStr := r.FormValue("principal_id")
	if principalIDStr == "" {
		h.renderScopes(w, r, "principal_id is required")
		return
	}
	principalID, err := uuid.Parse(principalIDStr)
	if err != nil {
		h.renderScopes(w, r, "invalid principal_id")
		return
	}
	if h.pool == nil {
		h.renderScopes(w, r, "service unavailable")
		return
	}
	if !h.hasScopeAdminAccess(r.Context(), r, id) {
		h.renderScopes(w, r, "scope admin required")
		return
	}
	if _, err := db.UpdateScopeOwner(r.Context(), h.pool, id, principalID); err != nil {
		h.renderScopes(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/ui/scopes", http.StatusSeeOther)
}

// handleCreatePrincipal serves POST /ui/principals.
func (h *Handler) handleCreatePrincipal(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderPrincipals(w, r, "bad form data", "", "", "")
		return
	}
	kind := r.FormValue("kind")
	slug := r.FormValue("slug")
	displayName := r.FormValue("display_name")
	if kind == "" || slug == "" || displayName == "" {
		h.renderPrincipals(w, r, "kind, slug and display_name are required", "", "", "")
		return
	}
	if h.pool == nil {
		h.renderPrincipals(w, r, "service unavailable", "", "", "")
		return
	}
	if !h.hasAnyPrincipalAdminRole(r.Context(), r) {
		h.renderPrincipals(w, r, "principal admin required", "", "", "")
		return
	}
	ps := principals.NewStore(h.pool)
	if _, err := ps.Create(r.Context(), kind, slug, displayName, nil); err != nil {
		h.renderPrincipals(w, r, err.Error(), "", "", "")
		return
	}
	http.Redirect(w, r, "/ui/principals", http.StatusSeeOther)
}

// handleUpdatePrincipal serves POST /ui/principals/{id}.
func (h *Handler) handleUpdatePrincipal(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/ui/principals/")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.renderPrincipals(w, r, "", "invalid principal id", "", "")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderPrincipals(w, r, "", "bad form data", "", "")
		return
	}
	slug := r.FormValue("slug")
	displayName := r.FormValue("display_name")
	if slug == "" || displayName == "" {
		h.renderPrincipals(w, r, "", "slug and display_name are required", "", "")
		return
	}
	if h.pool == nil {
		h.renderPrincipals(w, r, "", "service unavailable", "", "")
		return
	}
	if !h.hasPrincipalAdminAccess(r.Context(), r, id) {
		h.renderPrincipals(w, r, "", "principal admin required", "", "")
		return
	}
	ps := principals.NewStore(h.pool)
	if _, err := ps.UpdateProfile(r.Context(), id, slug, displayName); err != nil {
		h.renderPrincipals(w, r, "", err.Error(), "", "")
		return
	}
	http.Redirect(w, r, "/ui/principals", http.StatusSeeOther)
}

// handleCreateScope serves POST /ui/scopes.
func (h *Handler) handleCreateScope(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderScopes(w, r, "bad form data")
		return
	}
	kind := r.FormValue("kind")
	externalID := r.FormValue("external_id")
	name := r.FormValue("name")
	principalIDStr := r.FormValue("principal_id")
	parentIDStr := r.FormValue("parent_id")

	if kind == "" || externalID == "" || name == "" || principalIDStr == "" {
		h.renderScopes(w, r, "kind, external_id, name and principal are required")
		return
	}
	principalID, err := uuid.Parse(principalIDStr)
	if err != nil {
		h.renderScopes(w, r, "invalid principal id")
		return
	}
	var parentID *uuid.UUID
	if parentIDStr != "" {
		pid, err := uuid.Parse(parentIDStr)
		if err != nil {
			h.renderScopes(w, r, "invalid parent scope id")
			return
		}
		parentID = &pid
	}
	if h.pool == nil {
		h.renderScopes(w, r, "service unavailable")
		return
	}
	if parentID != nil && !h.hasScopeAdminAccess(r.Context(), r, *parentID) {
		h.renderScopes(w, r, "scope admin required")
		return
	}
	if _, err := db.CreateScope(r.Context(), h.pool, kind, externalID, name, parentID, principalID, nil); err != nil {
		h.renderScopes(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/ui/scopes", http.StatusSeeOther)
}

// handleSetScopeRepo serves POST /ui/scopes/{id}/repo.
func (h *Handler) handleSetScopeRepo(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/ui/scopes/")
	idStr := strings.TrimSuffix(trimmed, "/repo")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.renderScopes(w, r, "invalid scope id")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.renderScopes(w, r, "bad form data")
		return
	}
	repoURL := r.FormValue("repo_url")
	defaultBranch := r.FormValue("default_branch")
	if repoURL == "" {
		h.renderScopes(w, r, "repo_url is required")
		return
	}
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	if h.pool == nil {
		h.renderScopes(w, r, "service unavailable")
		return
	}
	if !h.hasScopeAdminAccess(r.Context(), r, id) {
		h.renderScopes(w, r, "scope admin required")
		return
	}
	if _, err := db.SetScopeRepo(r.Context(), h.pool, id, repoURL, defaultBranch); err != nil {
		h.renderScopes(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/ui/scopes", http.StatusSeeOther)
}

// handleSyncScopeRepo serves POST /ui/scopes/{id}/repo/sync.
// Starts a background sync and redirects immediately.
func (h *Handler) handleSyncScopeRepo(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/ui/scopes/")
	idStr := strings.TrimSuffix(trimmed, "/repo/sync")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.renderScopes(w, r, "invalid scope id")
		return
	}
	if h.pool == nil {
		h.renderScopes(w, r, "service unavailable")
		return
	}
	if !h.hasScopeAdminAccess(r.Context(), r, id) {
		h.renderScopes(w, r, "scope admin required")
		return
	}
	scope, err := db.GetScopeByID(r.Context(), h.pool, id)
	if err != nil || scope == nil {
		h.renderScopes(w, r, "scope not found")
		return
	}
	if scope.RepoUrl == nil || *scope.RepoUrl == "" {
		h.renderScopes(w, r, "no repository attached to this scope")
		return
	}
	_ = r.ParseForm()
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
		AuthToken:        r.FormValue("auth_token"),
		SSHKey:           r.FormValue("ssh_key"),
		SSHKeyPassphrase: r.FormValue("ssh_key_passphrase"),
		PrevCommit:       prevCommit,
	}
	h.syncer.Start(h.pool, opts) // fire and forget; status polled by UI
	http.Redirect(w, r, "/ui/scopes", http.StatusSeeOther)
}

// handleSyncStatus serves GET /ui/scopes/{id}/repo/sync/status.
// Returns JSON sync status for JS polling.
func (h *Handler) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/ui/scopes/")
	idStr := strings.TrimSuffix(trimmed, "/repo/sync/status")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid scope id", http.StatusBadRequest)
		return
	}
	status := h.syncer.Status(id)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

// handleAddMembership serves POST /ui/memberships.
func (h *Handler) handleAddMembership(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderPrincipals(w, r, "", "", "bad form data", "")
		return
	}
	memberIDStr := r.FormValue("member_id")
	parentIDStr := r.FormValue("parent_id")
	role := r.FormValue("role")
	if memberIDStr == "" || parentIDStr == "" || role == "" {
		h.renderPrincipals(w, r, "", "", "member, parent and role are required", "")
		return
	}
	memberID, err := uuid.Parse(memberIDStr)
	if err != nil {
		h.renderPrincipals(w, r, "", "", "invalid member id", "")
		return
	}
	parentID, err := uuid.Parse(parentIDStr)
	if err != nil {
		h.renderPrincipals(w, r, "", "", "invalid parent id", "")
		return
	}
	if h.pool == nil {
		h.renderPrincipals(w, r, "", "", "service unavailable", "")
		return
	}
	if !h.hasPrincipalAdminAccess(r.Context(), r, parentID) {
		h.renderPrincipals(w, r, "", "", "principal admin required", "")
		return
	}
	grantedBy := h.principalFromCookie(r)
	var grantedByPtr *uuid.UUID
	if grantedBy != uuid.Nil {
		grantedByPtr = &grantedBy
	}
	ms := principals.NewMembershipStore(h.pool)
	if err := ms.AddMembership(r.Context(), memberID, parentID, role, grantedByPtr); err != nil {
		h.renderPrincipals(w, r, "", "", err.Error(), "")
		return
	}
	http.Redirect(w, r, "/ui/principals", http.StatusSeeOther)
}

// handleDeleteMembership serves POST /ui/memberships/delete.
func (h *Handler) handleDeleteMembership(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderPrincipals(w, r, "", "", "bad form data", "")
		return
	}
	memberIDStr := r.FormValue("member_id")
	parentIDStr := r.FormValue("parent_id")
	memberID, err := uuid.Parse(memberIDStr)
	if err != nil {
		h.renderPrincipals(w, r, "", "", "invalid member id", "")
		return
	}
	parentID, err := uuid.Parse(parentIDStr)
	if err != nil {
		h.renderPrincipals(w, r, "", "", "invalid parent id", "")
		return
	}
	if h.pool == nil {
		h.renderPrincipals(w, r, "", "", "service unavailable", "")
		return
	}
	if !h.hasPrincipalAdminAccess(r.Context(), r, parentID) {
		h.renderPrincipals(w, r, "", "", "principal admin required", "")
		return
	}
	if err := db.DeleteMembership(r.Context(), h.pool, memberID, parentID); err != nil {
		h.renderPrincipals(w, r, "", "", err.Error(), "")
		return
	}
	http.Redirect(w, r, "/ui/principals", http.StatusSeeOther)
}

// handleCreateScopeGrant serves POST /ui/scope-grants.
func (h *Handler) handleCreateScopeGrant(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderPrincipals(w, r, "", "", "", "bad form data")
		return
	}
	scopeID, err := uuid.Parse(r.FormValue("scope_id"))
	if err != nil {
		h.renderPrincipals(w, r, "", "", "", "invalid scope id")
		return
	}
	granteeID, err := uuid.Parse(r.FormValue("principal_id"))
	if err != nil {
		h.renderPrincipals(w, r, "", "", "", "invalid principal id")
		return
	}
	rawPerms := append([]string{}, r.Form["permissions_basic"]...)
	rawPerms = append(rawPerms, r.Form["permissions_adv"]...)
	// Backward compatibility for older forms/tests.
	rawPerms = append(rawPerms, r.Form["permissions"]...)
	if len(rawPerms) == 0 {
		h.renderPrincipals(w, r, "", "", "", "at least one permission is required")
		return
	}
	perms, err := parseScopeGrantPermissionsInput(rawPerms)
	if err != nil {
		h.renderPrincipals(w, r, "", "", "", "invalid permissions: "+err.Error())
		return
	}
	if h.pool == nil {
		h.renderPrincipals(w, r, "", "", "", "service unavailable")
		return
	}
	if !h.hasScopePermission(r.Context(), r, scopeID, authz.NewPermission(authz.ResourceSharing, authz.OperationWrite)) {
		h.renderPrincipals(w, r, "", "", "", "sharing write required on scope")
		return
	}
	callerToken := h.tokenFromCookie(r)
	if callerToken == nil {
		h.renderPrincipals(w, r, "", "", "", "service unavailable")
		return
	}
	callerPrincipal, err := db.GetPrincipalByID(r.Context(), h.pool, callerToken.PrincipalID)
	if err != nil {
		h.renderPrincipals(w, r, "", "", "", err.Error())
		return
	}
	if callerPrincipal == nil || !callerPrincipal.IsSystemAdmin {
		tokenResolver := authz.NewTokenResolver(authz.NewDBResolver(h.pool))
		effective, err := tokenResolver.EffectiveTokenPermissions(r.Context(), callerToken, scopeID)
		if err != nil {
			h.renderPrincipals(w, r, "", "", "", "failed to resolve caller permissions")
			return
		}
		for _, p := range perms.Permissions() {
			if !effective.Contains(p) {
				h.renderPrincipals(w, r, "", "", "", "cannot grant permission "+string(p))
				return
			}
		}
	}

	grantedBy := h.principalFromCookie(r)
	var grantedByPtr *uuid.UUID
	if grantedBy != uuid.Nil {
		grantedByPtr = &grantedBy
	}
	canonicalPerms := make([]string, 0, len(perms.Permissions()))
	for _, p := range perms.Permissions() {
		canonicalPerms = append(canonicalPerms, string(p))
	}
	sort.Strings(canonicalPerms)

	q := db.New(h.pool)
	_, err = q.CreateScopeGrant(r.Context(), db.CreateScopeGrantParams{
		PrincipalID: granteeID,
		ScopeID:     scopeID,
		Permissions: canonicalPerms,
		GrantedBy:   grantedByPtr,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			_, err = q.UpdateScopeGrantPermissions(r.Context(), db.UpdateScopeGrantPermissionsParams{
				PrincipalID: granteeID,
				ScopeID:     scopeID,
				Permissions: canonicalPerms,
			})
		}
		if err != nil {
			h.renderPrincipals(w, r, "", "", "", err.Error())
			return
		}
	}
	http.Redirect(w, r, "/ui/principals", http.StatusSeeOther)
}

// handleDeleteScopeGrant serves POST /ui/scope-grants/delete.
func (h *Handler) handleDeleteScopeGrant(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderPrincipals(w, r, "", "", "", "bad form data")
		return
	}
	scopeID, err := uuid.Parse(r.FormValue("scope_id"))
	if err != nil {
		h.renderPrincipals(w, r, "", "", "", "invalid scope id")
		return
	}
	grantID, err := uuid.Parse(r.FormValue("grant_id"))
	if err != nil {
		h.renderPrincipals(w, r, "", "", "", "invalid grant id")
		return
	}
	if h.pool == nil {
		h.renderPrincipals(w, r, "", "", "", "service unavailable")
		return
	}
	if !h.hasScopePermission(r.Context(), r, scopeID, authz.NewPermission(authz.ResourceSharing, authz.OperationDelete)) {
		h.renderPrincipals(w, r, "", "", "", "sharing delete required on scope")
		return
	}
	q := db.New(h.pool)
	if err := q.DeleteScopeGrantByIDAndScope(r.Context(), db.DeleteScopeGrantByIDAndScopeParams{
		ID:      grantID,
		ScopeID: scopeID,
	}); err != nil {
		h.renderPrincipals(w, r, "", "", "", err.Error())
		return
	}
	http.Redirect(w, r, "/ui/principals", http.StatusSeeOther)
}

func parseScopeGrantPermissionsInput(raw []string) (authz.PermissionSet, error) {
	expanded := make([]string, 0, len(raw))
	for _, entry := range raw {
		value := strings.TrimSpace(entry)
		if value == "" {
			continue
		}
		if strings.Contains(value, ":") {
			expanded = append(expanded, value)
			continue
		}
		resource := authz.Resource(value)
		ops := authz.ValidOperations(resource)
		if len(ops) == 0 {
			return authz.PermissionSet{}, fmt.Errorf("unknown resource %q", value)
		}
		for _, op := range ops {
			expanded = append(expanded, string(authz.NewPermission(resource, op)))
		}
	}
	return authz.ParseTokenPermissions(expanded)
}
