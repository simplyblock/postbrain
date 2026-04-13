package ui

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
)

// handlePromotions serves GET /ui/promotions.
func (h *Handler) handlePromotions(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Scopes     []*db.Scope
		ScopeID    string
		Status     string
		Promotions []*db.PromotionRequest
	}{
		ScopeID: r.URL.Query().Get("scope_id"),
		Status:  r.URL.Query().Get("status"),
	}
	if data.Status == "" {
		data.Status = "all"
	}

	targetScopeID := uuid.Nil
	if data.ScopeID != "" {
		parsed, err := uuid.Parse(data.ScopeID)
		if err != nil {
			http.Error(w, "invalid scope id", http.StatusBadRequest)
			return
		}
		targetScopeID = parsed
	}
	validStatus := map[string]bool{
		"all":      true,
		"pending":  true,
		"approved": true,
		"rejected": true,
		"merged":   true,
	}
	if !validStatus[data.Status] {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}

	if h.pool != nil {
		scopes, scopeSet := h.authorizedScopesForRequest(r.Context(), r)
		data.Scopes = scopes
		if targetScopeID != uuid.Nil {
			if _, ok := scopeSet[targetScopeID]; !ok {
				data.Promotions = []*db.PromotionRequest{}
				h.render(w, r, "promotions", "Promotion Queue", data)
				return
			}
		}

		statusFilter := ""
		if data.Status != "all" {
			statusFilter = data.Status
		}
		proms, err := db.ListPromotions(r.Context(), h.pool, targetScopeID, statusFilter, 500)
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

// handleApprovePromotion serves POST /ui/promotions/{id}/approve.
func (h *Handler) handleApprovePromotion(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/ui/promotions/"), "/approve")
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
	http.Redirect(w, r, "/ui/promotions", http.StatusSeeOther)
}

// handleRejectPromotion serves POST /ui/promotions/{id}/reject.
func (h *Handler) handleRejectPromotion(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/ui/promotions/"), "/reject")
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
	http.Redirect(w, r, "/ui/promotions", http.StatusSeeOther)
}

// handleStaleness serves GET /ui/staleness.
func (h *Handler) handleStaleness(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Flags []*db.StalenessFlag
	}{}

	if h.pool != nil {
		_, scopeSet := h.authorizedScopesForRequest(r.Context(), r)
		flags, err := db.ListStalenessFlags(r.Context(), h.pool, "open", 50, 0)
		if err == nil {
			filtered := make([]*db.StalenessFlag, 0, len(flags))
			for _, f := range flags {
				art, getErr := db.GetArtifact(r.Context(), h.pool, f.ArtifactID)
				if getErr != nil || art == nil {
					continue
				}
				if _, ok := scopeSet[art.OwnerScopeID]; ok {
					filtered = append(filtered, f)
				}
			}
			data.Flags = filtered
		}
	}

	h.render(w, r, "staleness", "Staleness Flags", data)
}
