package ui

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
)

// handleTokens serves GET /ui/tokens.
func (h *Handler) handleTokens(w http.ResponseWriter, r *http.Request) {
	h.renderTokens(w, r, "", "")
}

func (h *Handler) renderTokens(w http.ResponseWriter, r *http.Request, formErr, newRawToken string) {
	data := struct {
		Tokens    []*db.Token
		Scopes    []*db.Scope
		FormError string
		NewToken  string // shown once after creation
	}{FormError: formErr, NewToken: newRawToken}

	if h.pool != nil {
		principalID := h.principalFromCookie(r)
		if principalID != uuid.Nil {
			tokens, err := db.ListTokens(r.Context(), h.pool, &principalID)
			if err == nil {
				data.Tokens = tokens
			}
		}
		scopes, err := db.ListScopes(r.Context(), h.pool, 200, 0)
		if err == nil {
			data.Scopes = scopes
		}
	}

	h.render(w, r, "tokens", "Token Management", data)
}

// handleCreateToken serves POST /ui/tokens.
func (h *Handler) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderTokens(w, r, "bad form data", "")
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		h.renderTokens(w, r, "name is required", "")
		return
	}

	// Parse optional scope IDs (multi-select).
	var scopeIDs []uuid.UUID
	for _, s := range r.Form["scope_ids"] {
		id, err := uuid.Parse(s)
		if err != nil {
			h.renderTokens(w, r, "invalid scope id: "+s, "")
			return
		}
		scopeIDs = append(scopeIDs, id)
	}

	// Parse optional expiry.
	var expiresAt *time.Time
	if exp := strings.TrimSpace(r.FormValue("expires_at")); exp != "" {
		t, err := time.Parse("2006-01-02", exp)
		if err != nil {
			h.renderTokens(w, r, "invalid expiry date (use YYYY-MM-DD)", "")
			return
		}
		t = t.UTC().Add(24*time.Hour - time.Second) // end of day
		expiresAt = &t
	}

	if h.pool == nil {
		h.renderTokens(w, r, "service unavailable", "")
		return
	}

	raw, hash, err := auth.GenerateToken()
	if err != nil {
		h.renderTokens(w, r, "failed to generate token", "")
		return
	}

	principalID := h.principalFromCookie(r)
	store := auth.NewTokenStore(h.pool)
	if _, err := store.Create(r.Context(), principalID, hash, name, scopeIDs, nil, expiresAt); err != nil {
		h.renderTokens(w, r, err.Error(), "")
		return
	}

	// Re-render with the raw token shown once — it is never stored.
	h.renderTokens(w, r, "", raw)
}

// handleRevokeToken serves POST /ui/tokens/{id}/revoke.
func (h *Handler) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/ui/tokens/"), "/revoke")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid token id", http.StatusBadRequest)
		return
	}
	if h.pool == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	principalID := h.principalFromCookie(r)
	if principalID == uuid.Nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	tokens, err := db.ListTokens(r.Context(), h.pool, &principalID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	owned := false
	for _, tok := range tokens {
		if tok.ID == id {
			owned = true
			break
		}
	}
	if !owned {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	store := auth.NewTokenStore(h.pool)
	if err := store.Revoke(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/ui/tokens", http.StatusSeeOther)
}
