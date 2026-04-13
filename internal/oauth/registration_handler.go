package oauth

import (
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"
)

// HandleRegister handles dynamic OAuth client registration.
func (s *Server) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Server.DynamicRegistration {
		http.NotFound(w, r)
		return
	}
	ip := clientIP(r.RemoteAddr)
	if !s.limiter.Allow(ip) {
		writeOAuthError(w, http.StatusTooManyRequests, "slow_down")
		return
	}

	var req struct {
		ClientName              string   `json:"client_name"`
		RedirectURIs            []string `json:"redirect_uris"`
		GrantTypes              []string `json:"grant_types"`
		TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata")
		return
	}
	if req.ClientName == "" || len(req.RedirectURIs) == 0 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata")
		return
	}
	if len(req.GrantTypes) == 0 {
		req.GrantTypes = []string{"authorization_code"}
	}
	isPublic := req.TokenEndpointAuthMethod == "" || req.TokenEndpointAuthMethod == "none"

	client, rawSecret, err := s.clients.Register(r.Context(), RegisterRequest{
		Name:         req.ClientName,
		RedirectURIs: req.RedirectURIs,
		GrantTypes:   req.GrantTypes,
		Scopes:       []string{},
		IsPublic:     isPublic,
		Meta:         []byte("{}"),
	})
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error")
		return
	}

	resp := map[string]any{
		"client_id":                  client.ClientID,
		"client_name":                client.Name,
		"redirect_uris":              client.RedirectURIs,
		"grant_types":                client.GrantTypes,
		"token_endpoint_auth_method": map[bool]string{true: "none", false: "client_secret_post"}[client.IsPublic],
	}
	if !client.IsPublic {
		resp["client_secret"] = rawSecret
	}
	writeJSON(w, http.StatusCreated, resp)
}

type registrationLimiter struct {
	mu   sync.Mutex
	now  func() time.Time
	hits map[string][]time.Time
}

func newRegistrationLimiter() *registrationLimiter {
	return &registrationLimiter{
		now:  time.Now,
		hits: map[string][]time.Time{},
	}
}

func (l *registrationLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	cutoff := now.Add(-time.Hour)
	pruned := make([]time.Time, 0, len(l.hits[ip]))
	for _, t := range l.hits[ip] {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	if len(pruned) >= 10 {
		l.hits[ip] = pruned
		return false
	}
	l.hits[ip] = append(pruned, now)
	return true
}

func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		if remoteAddr == "" {
			return "unknown"
		}
		return remoteAddr
	}
	if host == "" {
		return "unknown"
	}
	return host
}
