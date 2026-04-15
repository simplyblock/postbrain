package rest

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db/compat"
)

func (ro *Router) listPromotions(w http.ResponseWriter, r *http.Request) {
	scopeStr := r.URL.Query().Get("scope")
	var targetScopeID uuid.UUID
	if scopeStr != "" {
		kind, externalID, err := parseScopeString(scopeStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		scope, err := compat.GetScopeByExternalID(r.Context(), ro.pool, kind, externalID)
		if err != nil || scope == nil {
			writeError(w, http.StatusBadRequest, "scope not found")
			return
		}
		if err := ro.authorizeRequestedScope(r.Context(), scope.ID); err != nil {
			writeScopeAuthzError(w, r, scope.ID, err)
			return
		}
		targetScopeID = scope.ID
	}

	promotions, err := compat.ListPendingPromotions(r.Context(), ro.pool, targetScopeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"promotions": promotions})
}

func (ro *Router) approvePromotion(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid promotion request id")
		return
	}

	// Load request first so we can authorize against its TargetScopeID.
	req, err := compat.GetPromotionRequest(r.Context(), ro.pool, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if req == nil {
		writeError(w, http.StatusNotFound, "promotion request not found")
		return
	}
	if err := ro.authorizeScopeAdmin(r.Context(), req.TargetScopeID); err != nil {
		writeScopeAuthzError(w, r, req.TargetScopeID, err)
		return
	}

	reviewerID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	artifact, err := ro.knwProm.Approve(r.Context(), id, reviewerID, reviewerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, artifact)
}

type rejectPromotionRequest struct {
	Note *string `json:"note"`
}

func (ro *Router) rejectPromotion(w http.ResponseWriter, r *http.Request) {
	id, err := uuidParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid promotion request id")
		return
	}

	// Load request first so we can authorize against its TargetScopeID.
	req, err := compat.GetPromotionRequest(r.Context(), ro.pool, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if req == nil {
		writeError(w, http.StatusNotFound, "promotion request not found")
		return
	}
	if err := ro.authorizeScopeAdmin(r.Context(), req.TargetScopeID); err != nil {
		writeScopeAuthzError(w, r, req.TargetScopeID, err)
		return
	}

	var body rejectPromotionRequest
	_ = readJSON(r, &body)

	reviewerID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	if err := ro.knwProm.Reject(r.Context(), id, reviewerID, body.Note); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"promotion_request_id": id, "status": "rejected"})
}

func (ro *Router) registerPromotionRoutes(r chi.Router) {
	r.Get("/promotions", ro.listPromotions)
	r.Post("/promotions/{id}/approve", ro.approvePromotion)
	r.Post("/promotions/{id}/reject", ro.rejectPromotion)
}
