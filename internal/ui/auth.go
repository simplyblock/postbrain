package ui

import (
	"net/http"

	"github.com/simplyblock/postbrain/internal/auth"
)

const cookieName = "pb_session"

// authenticated checks the pb_session cookie. If missing or invalid, redirects to /ui/login.
// Returns true if the request is authenticated.
func (h *Handler) authenticated(w http.ResponseWriter, r *http.Request) bool {
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		http.Redirect(w, r, "/ui/login", http.StatusSeeOther)
		return false
	}
	// nil pool: reject all (e.g. in tests without a DB).
	if h.pool == nil {
		http.Redirect(w, r, "/ui/login", http.StatusSeeOther)
		return false
	}
	hash := auth.HashToken(cookie.Value)
	store := auth.NewTokenStore(h.pool)
	token, err := store.Lookup(r.Context(), hash)
	if err != nil || token == nil {
		http.Redirect(w, r, "/ui/login", http.StatusSeeOther)
		return false
	}
	return true
}

// handleLogin serves GET and POST /ui/login.
func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		raw := r.FormValue("token")
		if raw == "" {
			h.render(w, r, "login", "Login", map[string]any{"Error": "Token is required"})
			return
		}
		if h.pool == nil {
			h.render(w, r, "login", "Login", map[string]any{"Error": "Service unavailable"})
			return
		}
		hash := auth.HashToken(raw)
		store := auth.NewTokenStore(h.pool)
		token, err := store.Lookup(r.Context(), hash)
		if err != nil || token == nil {
			h.render(w, r, "login", "Login", map[string]any{"Error": "Invalid token"})
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    raw,
			Path:     "/ui",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, "/ui", http.StatusSeeOther)
		return
	}
	h.render(w, r, "login", "Login", map[string]any{})
}
