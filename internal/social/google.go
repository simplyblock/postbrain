package social

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/simplyblock/postbrain/internal/closeutil"
	"github.com/simplyblock/postbrain/internal/config"
)

type GoogleProvider struct {
	clientID     string
	clientSecret string
	scopes       []string
	redirectURI  string
	httpClient   *http.Client
	issuerURL    string // defaults to https://accounts.google.com; overridable for tests

	oidcOnce sync.Once
	oidcProv *oidc.Provider
	oidcErr  error
}

func NewGoogleProvider(baseURL string, cfg config.ProviderConfig) *GoogleProvider {
	return &GoogleProvider{
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		scopes:       cfg.Scopes,
		redirectURI:  strings.TrimSuffix(baseURL, "/") + "/ui/auth/google/callback",
		httpClient:   http.DefaultClient,
		issuerURL:    "https://accounts.google.com",
	}
}

func (p *GoogleProvider) AuthURL(state string) string {
	q := url.Values{}
	q.Set("client_id", p.clientID)
	q.Set("redirect_uri", p.redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", strings.Join(p.scopes, " "))
	q.Set("state", state)
	// Always use the canonical Google authorization endpoint.
	return "https://accounts.google.com/o/oauth2/v2/auth?" + q.Encode()
}

// provider returns the cached OIDC provider, initializing it on first call.
// The OIDC provider fetches Google's discovery document and caches the JWKS URI.
func (p *GoogleProvider) provider(ctx context.Context) (*oidc.Provider, error) {
	p.oidcOnce.Do(func() {
		// Inject our HTTP client so tests can intercept the discovery request.
		ctx = oidc.ClientContext(ctx, p.httpClient)
		p.oidcProv, p.oidcErr = oidc.NewProvider(ctx, p.issuerURL)
	})
	return p.oidcProv, p.oidcErr
}

// Exchange redeems the authorization code for an id_token, verifies its
// signature against Google's JWKS, and returns the verified user claims.
func (p *GoogleProvider) Exchange(ctx context.Context, code string) (*UserInfo, error) {
	prov, err := p.provider(ctx)
	if err != nil {
		return nil, fmt.Errorf("google oidc provider: %w", err)
	}

	// Exchange authorization code → id_token using the OIDC-discovered token endpoint.
	tokenURL := prov.Endpoint().TokenURL
	rawIDToken, err := p.exchangeIDToken(ctx, tokenURL, code)
	if err != nil {
		return nil, err
	}

	// Inject the custom HTTP client so that JWKS fetch/refresh during Verify
	// uses the same transport as provider discovery (required for tests and
	// custom timeouts/proxies; without this, go-oidc falls back to http.DefaultClient).
	verifyCtx := oidc.ClientContext(ctx, p.httpClient)

	// Verify signature, issuer (accounts.google.com), audience (our client_id),
	// and expiry. Rejects alg:none and any token with an invalid or missing signature.
	verifier := prov.Verifier(&oidc.Config{ClientID: p.clientID})
	verified, err := verifier.Verify(verifyCtx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("google id_token verification: %w", err)
	}

	var claims map[string]any
	if err := verified.Claims(&claims); err != nil {
		return nil, fmt.Errorf("google id_token claims: %w", err)
	}
	rawClaims, err := json.Marshal(claims)
	if err != nil {
		return nil, fmt.Errorf("google id_token marshal: %w", err)
	}

	return &UserInfo{
		ProviderID:    stringFromAny(claims["sub"]),
		Email:         stringFromAny(claims["email"]),
		EmailVerified: boolFromAny(claims["email_verified"]),
		HostedDomain:  stringFromAny(claims["hd"]),
		DisplayName:   stringFromAny(claims["name"]),
		AvatarURL:     stringFromAny(claims["picture"]),
		RawProfile:    rawClaims,
	}, nil
}

func (p *GoogleProvider) exchangeIDToken(ctx context.Context, tokenEndpoint, code string) (string, error) {
	form := url.Values{}
	form.Set("client_id", p.clientID)
	form.Set("client_secret", p.clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", p.redirectURI)
	form.Set("grant_type", "authorization_code")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer closeutil.Log(res.Body, "social: google token exchange: close response body")
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("google token exchange status=%d", res.StatusCode)
	}
	var body struct {
		IDToken string `json:"id_token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.IDToken == "" {
		return "", fmt.Errorf("google token exchange missing id_token")
	}
	return body.IDToken, nil
}

func stringFromAny(v any) string {
	s, _ := v.(string)
	return s
}

func boolFromAny(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true")
	default:
		return false
	}
}
