package rest

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db/compat"
)

type createPrincipalRequest struct {
	Kind        string `json:"kind"`
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Meta        []byte `json:"meta"`
}

func (ro *Router) listPrincipals(w http.ResponseWriter, r *http.Request) {
	pg := paginationFromRequest(r)
	ps, err := ro.principals.List(r.Context(), pg.Limit, pg.Offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"principals": ps})
}

func (ro *Router) createPrincipal(w http.ResponseWriter, r *http.Request) {
	var body createPrincipalRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Kind == "" || body.Slug == "" || body.DisplayName == "" {
		writeError(w, http.StatusBadRequest, "kind, slug and display_name are required")
		return
	}
	if body.Meta == nil {
		body.Meta = []byte("{}")
	}
	callerID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	allowed, err := ro.membership.HasAnyAdminRole(r.Context(), callerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "forbidden: principal admin required")
		return
	}
	p, err := ro.principals.Create(r.Context(), body.Kind, body.Slug, body.DisplayName, body.Meta)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (ro *Router) getPrincipal(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid principal id")
		return
	}
	p, err := ro.principals.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "principal not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type updatePrincipalRequest struct {
	DisplayName string `json:"display_name"`
	Meta        []byte `json:"meta"`
}

func (ro *Router) updatePrincipal(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid principal id")
		return
	}
	var body updatePrincipalRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	callerID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	allowed, err := ro.membership.IsPrincipalAdmin(r.Context(), callerID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "forbidden: principal admin required")
		return
	}
	p, err := ro.principals.Update(r.Context(), id, body.DisplayName, body.Meta)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (ro *Router) deletePrincipal(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid principal id")
		return
	}
	callerID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	allowed, err := ro.membership.IsPrincipalAdmin(r.Context(), callerID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "forbidden: principal admin required")
		return
	}
	if err := ro.principals.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (ro *Router) listMembers(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid principal id")
		return
	}
	callerID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	allowed, err := ro.membership.IsPrincipalAdmin(r.Context(), callerID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "forbidden: principal admin required")
		return
	}
	members, err := compat.GetMemberships(r.Context(), ro.pool, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": members})
}

type addMemberRequest struct {
	MemberID string `json:"member_id"`
	Role     string `json:"role"`
}

func (ro *Router) addMember(w http.ResponseWriter, r *http.Request) {
	parentID, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid principal id")
		return
	}
	var body addMemberRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	memberID, err := uuid.Parse(body.MemberID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid member_id")
		return
	}
	if body.Role == "" {
		body.Role = "member"
	}
	callerID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	allowed, err := ro.membership.IsPrincipalAdmin(r.Context(), callerID, parentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "forbidden: principal admin required")
		return
	}
	grantedBy, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	if err := ro.membership.AddMembership(r.Context(), memberID, parentID, body.Role, &grantedBy); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"member_id": memberID,
		"parent_id": parentID,
		"role":      body.Role,
	})
}

func (ro *Router) removeMember(w http.ResponseWriter, r *http.Request) {
	parentID, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid principal id")
		return
	}
	memberID, err := uuidParam(r, "member_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid member_id")
		return
	}
	callerID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	allowed, err := ro.membership.IsPrincipalAdmin(r.Context(), callerID, parentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "forbidden: principal admin required")
		return
	}
	if err := ro.membership.RemoveMembership(r.Context(), memberID, parentID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (ro *Router) registerPrincipalRoutes(r chi.Router) {
	r.Get("/principals", ro.listPrincipals)
	r.Post("/principals", ro.createPrincipal)
	r.Get("/principals/{id}", ro.getPrincipal)
	r.Put("/principals/{id}", ro.updatePrincipal)
	r.Delete("/principals/{id}", ro.deletePrincipal)
	r.Get("/principals/{id}/members", ro.listMembers)
	r.Post("/principals/{id}/members", ro.addMember)
	r.Delete("/principals/{id}/members/{member_id}", ro.removeMember)
}
