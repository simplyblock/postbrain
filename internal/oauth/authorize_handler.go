package oauth

import (
	"net/http"
	"net/url"
)

// HandleAuthorize validates the request and redirects to consent UI.
func (s *Server) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	clientID := q.Get("client_id")
	if clientID == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if q.Get("response_type") != "code" {
		writeOAuthError(w, http.StatusBadRequest, "unsupported_response_type")
		return
	}
	redirectURI := q.Get("redirect_uri")
	if redirectURI == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	state := q.Get("state")
	if state == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	rawScopes := q.Get("scope")
	scopes, err := ParseScopes(rawScopes)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_scope")
		return
	}
	client, err := s.clients.LookupByClientID(r.Context(), clientID)
	if err != nil || client == nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client")
		return
	}
	if err := s.clients.ValidateRedirectURI(client, redirectURI); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	codeChallenge := q.Get("code_challenge")
	codeChallengeMethod := q.Get("code_challenge_method")
	if client.IsPublic {
		if codeChallenge == "" || codeChallengeMethod != "S256" {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request")
			return
		}
	}
	if codeChallengeMethod != "" && codeChallengeMethod != "S256" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	rawConsentState, err := s.states.Issue(r.Context(), "mcp_consent", map[string]any{
		"client_id":      clientID,
		"redirect_uri":   redirectURI,
		"scopes":         scopes,
		"code_challenge": codeChallenge,
		"state":          state,
	}, s.cfg.Server.StateTTL)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error")
		return
	}

	consentURL := "/ui/oauth/authorize?state=" + url.QueryEscape(rawConsentState)
	http.Redirect(w, r, consentURL, http.StatusFound)
}
