package ui

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/authz"
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
		scopes, _ := h.effectivePrincipalScopesForRequest(r.Context(), r)
		data.Scopes = scopes
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
	permissions, err := authz.ParseTokenPermissions(r.Form["permissions"])
	if err != nil {
		h.renderTokens(w, r, err.Error(), "")
		return
	}
	permStrings := permissions.Permissions()
	rawPerms := make([]string, len(permStrings))
	for i, p := range permStrings {
		rawPerms[i] = string(p)
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
	if _, err := store.Create(r.Context(), principalID, hash, name, scopeIDs, rawPerms, expiresAt); err != nil {
		h.renderTokens(w, r, err.Error(), "")
		return
	}

	// Re-render with the raw token shown once — it is never stored.
	h.renderTokens(w, r, "", raw)
}


func parseScopeIDs(values []string) ([]uuid.UUID, error) {
	scopeIDs := make([]uuid.UUID, 0, len(values))
	seen := make(map[uuid.UUID]struct{}, len(values))
	for _, s := range values {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		scopeIDs = append(scopeIDs, id)
	}
	return scopeIDs, nil
}

func (h *Handler) ownedTokenByID(ctx context.Context, principalID, tokenID uuid.UUID) (*db.Token, error) {
	tokens, err := db.ListTokens(ctx, h.pool, &principalID)
	if err != nil {
		return nil, err
	}
	for _, tok := range tokens {
		if tok.ID == tokenID {
			return tok, nil
		}
	}
	return nil, nil
}

// handleUpdateTokenScopes serves POST /ui/tokens/{id}/scopes.
func (h *Handler) handleUpdateTokenScopes(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/ui/tokens/"), "/scopes")
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
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form data", http.StatusBadRequest)
		return
	}
	scopeIDs, err := parseScopeIDs(r.Form["scope_ids"])
	if err != nil {
		http.Error(w, "invalid scope id", http.StatusBadRequest)
		return
	}

	ownedToken, err := h.ownedTokenByID(r.Context(), principalID, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if ownedToken == nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	ok, err := db.UpdateTokenScopes(r.Context(), h.pool, id, principalID, scopeIDs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	http.Redirect(w, r, "/ui/tokens", http.StatusSeeOther)
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
	ownedToken, err := h.ownedTokenByID(r.Context(), principalID, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if ownedToken == nil {
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
