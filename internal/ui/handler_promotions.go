package ui

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
)

// handlePromotions serves GET /ui/{scope}/promotions.
func (h *Handler) handlePromotions(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "all"
	}

	validStatus := map[string]bool{
		"all": true, "pending": true, "approved": true, "rejected": true, "merged": true,
	}
	if !validStatus[status] {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}

	scope := scopeFromContext(r.Context())

	data := struct {
		ScopeID    string
		Status     string
		Promotions []*db.PromotionRequest
	}{
		Status: status,
	}
	if scope != nil {
		data.ScopeID = scope.ID.String()
	}

	if h.pool != nil && scope != nil {
		_, scopeSet := h.authorizedScopesForRequest(r.Context(), r)

		statusFilter := ""
		if status != "all" {
			statusFilter = status
		}
		proms, err := compat.ListPromotions(r.Context(), h.pool, scope.ID, statusFilter, 500)
		if err != nil {
			http.Error(w, "failed to load promotions", http.StatusInternalServerError)
			return
		}
		filtered := make([]*db.PromotionRequest, 0, len(proms))
		for _, p := range proms {
			if _, ok := scopeSet[p.TargetScopeID]; ok {
				filtered = append(filtered, p)
			}
		}
		data.Promotions = filtered
	}

	h.render(w, r, "promotions", "Promotion Queue", data)
}

// handleApprovePromotion serves POST /ui/{scope}/promotions/{id}/approve.
func (h *Handler) handleApprovePromotion(w http.ResponseWriter, r *http.Request) {
	path := routePathFromContext(r.Context(), r)
	idStr := strings.TrimSuffix(strings.TrimPrefix(path, "/ui/promotions/"), "/approve")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid promotion id", http.StatusBadRequest)
		return
	}
	reviewerID := h.principalFromCookie(r)
	if _, err := h.knwProm.Approve(r.Context(), id, reviewerID, reviewerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	scopedRedirect(w, r, "/promotions")
}

// handleRejectPromotion serves POST /ui/{scope}/promotions/{id}/reject.
func (h *Handler) handleRejectPromotion(w http.ResponseWriter, r *http.Request) {
	path := routePathFromContext(r.Context(), r)
	idStr := strings.TrimSuffix(strings.TrimPrefix(path, "/ui/promotions/"), "/reject")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid promotion id", http.StatusBadRequest)
		return
	}
	reviewerID := h.principalFromCookie(r)
	if err := h.knwProm.Reject(r.Context(), id, reviewerID, nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	scopedRedirect(w, r, "/promotions")
}

// handleStaleness serves GET /ui/{scope}/staleness.
func (h *Handler) handleStaleness(w http.ResponseWriter, r *http.Request) {
	scope := scopeFromContext(r.Context())

	data := struct {
		ScopeID string
		Flags   []*db.StalenessFlag
	}{}
	if scope != nil {
		data.ScopeID = scope.ID.String()
	}

	if h.pool != nil && scope != nil {
		flags, err := compat.ListStalenessFlags(r.Context(), h.pool, "open", 50, 0)
		if err == nil {
			filtered := make([]*db.StalenessFlag, 0, len(flags))
			for _, f := range flags {
				art, getErr := compat.GetArtifact(r.Context(), h.pool, f.ArtifactID)
				if getErr != nil || art == nil {
					continue
				}
				if art.OwnerScopeID == scope.ID {
					filtered = append(filtered, f)
				}
			}
			data.Flags = filtered
		}
	}

	h.render(w, r, "staleness", "Staleness Flags", data)
}
