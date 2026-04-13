package oauth

import (
	"errors"
	"net/http"
	"strings"
)

// HandleToken exchanges authorization code to bearer token.
func (s *Server) HandleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if r.Form.Get("grant_type") != "authorization_code" {
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type")
		return
	}

	rawCode := r.Form.Get("code")
	if rawCode == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	clientID := r.Form.Get("client_id")
	redirectURI := r.Form.Get("redirect_uri")
	verifier := r.Form.Get("code_verifier")

	client, err := s.clients.LookupByClientID(r.Context(), clientID)
	if err != nil || client == nil {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_client")
		return
	}

	code, err := s.codes.Consume(r.Context(), rawCode)
	if err != nil {
		if errors.Is(err, ErrCodeNotFound) || errors.Is(err, ErrCodeUsed) || errors.Is(err, ErrCodeExpired) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant")
			return
		}
		writeOAuthError(w, http.StatusInternalServerError, "server_error")
		return
	}
	if code.RedirectURI != redirectURI || code.ClientID != client.ID {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant")
		return
	}

	if !client.IsPublic {
		clientSecret := r.Form.Get("client_secret")
		if clientSecret == "" || client.ClientSecretHash == nil || hashSHA256Hex(clientSecret) != *client.ClientSecretHash {
			writeOAuthError(w, http.StatusUnauthorized, "invalid_client")
			return
		}
	}

	if client.IsPublic {
		if err := s.codes.VerifyPKCE(code, verifier); err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant")
			return
		}
	} else if verifier != "" && code.CodeChallenge != "" {
		if err := s.codes.VerifyPKCE(code, verifier); err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant")
			return
		}
	}

	rawToken, err := s.issuer.Issue(r.Context(), code.PrincipalID, code.Scopes, s.cfg.Server.TokenTTL)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error")
		return
	}

	expiresIn := int64(0)
	if s.cfg.Server.TokenTTL > 0 {
		expiresIn = int64(s.cfg.Server.TokenTTL.Seconds())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": rawToken,
		"token_type":   "Bearer",
		"expires_in":   expiresIn,
		"scope":        strings.Join(code.Scopes, " "),
	})
}
