package rest

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
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
		scope, err := db.GetScopeByExternalID(r.Context(), ro.pool, kind, externalID)
		if err != nil || scope == nil {
			writeError(w, http.StatusBadRequest, "scope not found")
			return
		}
		if err := ro.authorizeRequestedScope(r.Context(), scope.ID); err != nil {
			writeScopeAuthzError(w, err)
			return
		}
		targetScopeID = scope.ID
	}

	promotions, err := db.ListPendingPromotions(r.Context(), ro.pool, targetScopeID)
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
	var body rejectPromotionRequest
	_ = readJSON(r, &body)

	reviewerID, _ := r.Context().Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	if err := ro.knwProm.Reject(r.Context(), id, reviewerID, body.Note); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"promotion_request_id": id, "status": "rejected"})
}
