package ui

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/oauth"
)

func (h *Handler) handleConsentGet(w http.ResponseWriter, r *http.Request) {
	if h.stateStore == nil {
		http.Error(w, "oauth state store unavailable", http.StatusServiceUnavailable)
		return
	}
	rawState := r.URL.Query().Get("state")
	if rawState == "" {
		http.Error(w, "missing oauth state", http.StatusBadRequest)
		return
	}
	state, err := h.stateStore.Peek(r.Context(), rawState)
	if err != nil {
		if errors.Is(err, oauth.ErrNotFound) {
			http.Error(w, "invalid oauth state", http.StatusBadRequest)
			return
		}
		http.Error(w, "failed to load oauth state", http.StatusInternalServerError)
		return
	}
	payload := parseConsentPayload(state.Payload)
	clientName := payload.ClientID
	if h.clients != nil {
		if client, err := h.clients.LookupByClientID(r.Context(), payload.ClientID); err == nil && client != nil {
			clientName = client.Name
		}
	}
	h.render(w, r, "oauth_consent", "OAuth Consent", struct {
		State      string
		ClientName string
		Scopes     []string
	}{State: rawState, ClientName: clientName, Scopes: payload.Scopes})
}

func (h *Handler) handleConsentPost(w http.ResponseWriter, r *http.Request) {
	if h.stateStore == nil || h.codeStore == nil || h.clients == nil {
		http.Error(w, "oauth dependencies unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	rawState := r.FormValue("state")
	if rawState == "" {
		http.Error(w, "missing oauth state", http.StatusBadRequest)
		return
	}
	state, err := h.stateStore.Consume(r.Context(), rawState)
	if err != nil {
		if errors.Is(err, oauth.ErrNotFound) {
			http.Error(w, "invalid oauth state", http.StatusBadRequest)
			return
		}
		http.Error(w, "failed to consume oauth state", http.StatusInternalServerError)
		return
	}
	payload := parseConsentPayload(state.Payload)

	if r.FormValue("action") != "approve" {
		http.Redirect(w, r, withQuery(payload.RedirectURI, map[string]string{
			"error": "access_denied",
			"state": payload.OriginalState,
		}), http.StatusFound)
		return
	}

	client, err := h.clients.LookupByClientID(r.Context(), payload.ClientID)
	if err != nil || client == nil {
		http.Error(w, "invalid oauth client", http.StatusBadRequest)
		return
	}

	principalID := principalIDFromRequest(r)
	if principalID == uuid.Nil {
		principalID = h.principalFromCookie(r)
	}
	if principalID == uuid.Nil {
		http.Error(w, "missing principal", http.StatusBadRequest)
		return
	}

	rawCode, err := h.codeStore.Issue(r.Context(), oauth.IssueCodeRequest{
		ClientID:      client.ID,
		PrincipalID:   principalID,
		RedirectURI:   payload.RedirectURI,
		Scopes:        payload.Scopes,
		CodeChallenge: payload.CodeChallenge,
		TTL:           h.oauthCfg.Server.AuthCodeTTL,
	})
	if err != nil {
		http.Error(w, "failed to issue oauth code", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, withQuery(payload.RedirectURI, map[string]string{
		"code":  rawCode,
		"state": payload.OriginalState,
	}), http.StatusFound)
}

type consentPayload struct {
	ClientID      string
	RedirectURI   string
	Scopes        []string
	CodeChallenge string
	OriginalState string
}

func parseConsentPayload(raw map[string]any) consentPayload {
	p := consentPayload{
		ClientID:      anyString(raw["client_id"]),
		RedirectURI:   anyString(raw["redirect_uri"]),
		CodeChallenge: anyString(raw["code_challenge"]),
		OriginalState: anyString(raw["state"]),
	}
	if scopes, ok := raw["scopes"].([]any); ok {
		for _, s := range scopes {
			if scope := anyString(s); scope != "" {
				p.Scopes = append(p.Scopes, scope)
			}
		}
	}
	if scopes, ok := raw["scopes"].([]string); ok {
		p.Scopes = append([]string{}, scopes...)
	}
	return p
}

func withQuery(base string, values map[string]string) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	q := u.Query()
	for k, v := range values {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func principalIDFromRequest(r *http.Request) uuid.UUID {
	v := r.Context().Value(auth.ContextKeyPrincipalID)
	id, ok := v.(uuid.UUID)
	if !ok {
		return uuid.Nil
	}
	return id
}

func anyString(v any) string {
	s, _ := v.(string)
	return s
}
