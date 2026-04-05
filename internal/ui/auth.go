package ui

import (
	"net/http"
	"sort"

	"github.com/simplyblock/postbrain/internal/auth"
)

const cookieName = "pb_session"

// authenticated checks the pb_session cookie. If missing or invalid, redirects to /ui/login.
// Returns true if the request is authenticated.
func (h *Handler) authenticated(w http.ResponseWriter, r *http.Request) bool {
	return h.authenticatedRedirect(w, r, "/ui/login")
}

func (h *Handler) authenticatedRedirect(w http.ResponseWriter, r *http.Request, loginTarget string) bool {
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		http.Redirect(w, r, loginTarget, http.StatusSeeOther)
		return false
	}
	// nil pool: reject all (e.g. in tests without a DB).
	if h.pool == nil {
		http.Redirect(w, r, loginTarget, http.StatusSeeOther)
		return false
	}
	hash := auth.HashToken(cookie.Value)
	store := auth.NewTokenStore(h.pool)
	token, err := store.Lookup(r.Context(), hash)
	if err != nil || token == nil {
		http.Redirect(w, r, loginTarget, http.StatusSeeOther)
		return false
	}
	return true
}

// handleLogin serves GET and POST /ui/login.
func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	renderLogin := func(errMsg string, next string) {
		providers := make([]string, 0, len(h.providers))
		for name := range h.providers {
			providers = append(providers, name)
		}
		sort.Strings(providers)
		h.render(w, r, "login", "Login", map[string]any{
			"Error":     errMsg,
			"Next":      next,
			"Providers": providers,
		})
	}

	next := r.URL.Query().Get("next")
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if formNext := r.FormValue("next"); formNext != "" {
			next = formNext
		}
		raw := r.FormValue("token")
		if raw == "" {
			renderLogin("Token is required", next)
			return
		}
		if h.pool == nil {
			renderLogin("Service unavailable", next)
			return
		}
		hash := auth.HashToken(raw)
		store := auth.NewTokenStore(h.pool)
		token, err := store.Lookup(r.Context(), hash)
		if err != nil || token == nil {
			renderLogin("Invalid token", next)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    raw,
			Path:     "/ui",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		redirectTarget := "/ui"
		if next != "" {
			redirectTarget = next
		}
		http.Redirect(w, r, redirectTarget, http.StatusSeeOther)
		return
	}
	renderLogin("", next)
}

// handleLogout serves POST /ui/logout.
func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/ui",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/ui/login", http.StatusSeeOther)
}
