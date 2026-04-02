package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
)

type serverClientStore interface {
	LookupByClientID(ctx context.Context, clientID string) (*OAuthClient, error)
	ValidateRedirectURI(client *OAuthClient, redirectURI string) error
	Register(ctx context.Context, req RegisterRequest) (*OAuthClient, string, error)
}

type serverCodeStore interface {
	Consume(ctx context.Context, rawCode string) (*AuthCode, error)
	VerifyPKCE(code *AuthCode, verifier string) error
}

type serverStateStore interface {
	Issue(ctx context.Context, kind string, payload map[string]any, ttl time.Duration) (string, error)
}

type serverTokenStore interface {
	Lookup(ctx context.Context, hash string) (*db.Token, error)
	Revoke(ctx context.Context, tokenID uuid.UUID) error
}

type serverIssuer interface {
	Issue(ctx context.Context, principalID uuid.UUID, scopes []string, ttl time.Duration) (string, error)
}

// Server hosts OAuth authorization server routes.
type Server struct {
	clients serverClientStore
	codes   serverCodeStore
	states  serverStateStore
	issuer  serverIssuer
	tokens  serverTokenStore
	cfg     config.OAuthConfig
	limiter *registrationLimiter
}

// NewServer constructs an OAuth server.
func NewServer(clients serverClientStore, codes serverCodeStore, states serverStateStore, issuer serverIssuer, tokens serverTokenStore, cfg config.OAuthConfig) *Server {
	return &Server{
		clients: clients,
		codes:   codes,
		states:  states,
		issuer:  issuer,
		tokens:  tokens,
		cfg:     cfg,
		limiter: newRegistrationLimiter(),
	}
}

// HandleMetadata serves RFC 8414 metadata.
func (s *Server) HandleMetadata(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, ServerMetadata(s.cfg.BaseURL))
}

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

	if err := s.codes.VerifyPKCE(code, verifier); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant")
		return
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

// HandleRevoke revokes token when it exists. Unknown tokens still return 200.
func (s *Server) HandleRevoke(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusOK, "")
		return
	}
	rawToken := r.Form.Get("token")
	if rawToken != "" {
		sum := sha256.Sum256([]byte(rawToken))
		hash := hex.EncodeToString(sum[:])
		token, err := s.tokens.Lookup(r.Context(), hash)
		if err == nil && token != nil {
			_ = s.tokens.Revoke(r.Context(), token.ID)
		}
	}
	w.WriteHeader(http.StatusOK)
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

func writeOAuthError(w http.ResponseWriter, status int, code string) {
	if code == "" {
		w.WriteHeader(status)
		return
	}
	writeJSON(w, status, map[string]any{"error": code})
}

func writeJSON(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
