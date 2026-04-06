package ui

import (
	"errors"
	"net/http"
	"strings"

	"github.com/simplyblock/postbrain/internal/oauth"
	"github.com/simplyblock/postbrain/internal/social"
)

func (h *Handler) handleSocialStart(w http.ResponseWriter, r *http.Request) {
	providerName := socialProviderFromPath(r.URL.Path)
	provider, ok := h.providers[providerName]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if h.stateStore == nil {
		http.Error(w, "oauth state store unavailable", http.StatusServiceUnavailable)
		return
	}

	rawState, err := h.stateStore.Issue(r.Context(), "social", map[string]any{}, h.oauthCfg.Server.StateTTL)
	if err != nil {
		http.Error(w, "failed to create oauth state", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, provider.AuthURL(rawState), http.StatusFound)
}

func (h *Handler) handleSocialCallback(w http.ResponseWriter, r *http.Request) {
	providerName := socialProviderFromPath(r.URL.Path)
	provider, ok := h.providers[providerName]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if h.stateStore == nil || h.identities == nil || h.issuer == nil {
		http.Error(w, "oauth dependencies unavailable", http.StatusServiceUnavailable)
		return
	}

	rawState := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if rawState == "" || code == "" {
		http.Error(w, "invalid oauth callback", http.StatusBadRequest)
		return
	}

	if _, err := h.stateStore.Consume(r.Context(), rawState); err != nil {
		if errors.Is(err, oauth.ErrNotFound) {
			http.Error(w, "invalid oauth state", http.StatusBadRequest)
			return
		}
		http.Error(w, "failed to validate oauth state", http.StatusInternalServerError)
		return
	}

	info, err := provider.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, "failed to exchange oauth code", http.StatusBadGateway)
		return
	}

	if h.oauthCfg.Social.RequireVerifiedEmail && !info.EmailVerified {
		http.Error(w, "email is not verified", http.StatusForbidden)
		return
	}
	if !isAllowedEmailDomain(info.Email, info.HostedDomain, h.oauthCfg.Social.AllowedEmailDomains) {
		http.Error(w, "email domain is not allowed", http.StatusForbidden)
		return
	}

	principal, err := h.identities.FindOrCreateWithPolicy(
		r.Context(),
		providerName,
		info,
		social.IdentityPolicy{AutoCreateUsers: h.oauthCfg.Social.AutoCreateUsers},
	)
	if err != nil {
		if errors.Is(err, social.ErrPrincipalNotProvisioned) {
			http.Error(w, "account is not provisioned", http.StatusForbidden)
			return
		}
		http.Error(w, "failed to link social identity", http.StatusInternalServerError)
		return
	}

	rawToken, err := h.issuer.Issue(r.Context(), principal.ID, socialLoginScopes(), h.oauthCfg.Server.TokenTTL)
	if err != nil {
		http.Error(w, "failed to issue session token", http.StatusInternalServerError)
		return
	}

	c := &http.Cookie{
		Name:     cookieName,
		Value:    rawToken,
		Path:     "/ui",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	if strings.HasPrefix(strings.ToLower(h.oauthCfg.BaseURL), "https://") {
		c.Secure = true
	}
	http.SetCookie(w, c)
	http.Redirect(w, r, "/ui", http.StatusSeeOther)
}

func isAllowedEmailDomain(email, hostedDomain string, allowedDomains []string) bool {
	if len(allowedDomains) == 0 {
		return true
	}

	emailDomain := ""
	if at := strings.LastIndex(email, "@"); at >= 0 && at+1 < len(email) {
		emailDomain = strings.ToLower(strings.TrimSpace(email[at+1:]))
	}
	hd := strings.ToLower(strings.TrimSpace(hostedDomain))

	for _, d := range allowedDomains {
		want := strings.ToLower(strings.TrimSpace(d))
		if want == "" {
			continue
		}
		if emailDomain == want || hd == want {
			return true
		}
	}
	return false
}

func socialProviderFromPath(path string) string {
	path = strings.TrimPrefix(path, "/ui/auth/")
	path = strings.TrimSuffix(path, "/callback")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func socialLoginScopes() []string {
	return []string{
		oauth.ScopeMemoriesRead,
		oauth.ScopeMemoriesWrite,
		oauth.ScopeKnowledgeRead,
		oauth.ScopeKnowledgeWrite,
		oauth.ScopeSkillsRead,
		oauth.ScopeSkillsWrite,
		oauth.ScopePrincipalsRead,
		oauth.ScopeTokensRead,
		oauth.ScopeTokensWrite,
	}
}
