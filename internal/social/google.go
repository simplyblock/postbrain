package social

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/simplyblock/postbrain/internal/closeutil"
	"github.com/simplyblock/postbrain/internal/config"
)

type GoogleProvider struct {
	clientID     string
	clientSecret string
	scopes       []string
	redirectURI  string
	httpClient   *http.Client
	authorizeURL string
	discoveryURL string
}

func NewGoogleProvider(baseURL string, cfg config.ProviderConfig) *GoogleProvider {
	return &GoogleProvider{
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		scopes:       cfg.Scopes,
		redirectURI:  strings.TrimSuffix(baseURL, "/") + "/ui/auth/google/callback",
		httpClient:   http.DefaultClient,
		authorizeURL: "https://accounts.google.com/o/oauth2/v2/auth",
		discoveryURL: "https://accounts.google.com/.well-known/openid-configuration",
	}
}

func (p *GoogleProvider) AuthURL(state string) string {
	q := url.Values{}
	q.Set("client_id", p.clientID)
	q.Set("redirect_uri", p.redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", strings.Join(p.scopes, " "))
	q.Set("state", state)
	return p.authorizeURL + "?" + q.Encode()
}

func (p *GoogleProvider) Exchange(ctx context.Context, code string) (*UserInfo, error) {
	tokenEndpoint, err := p.fetchTokenEndpoint(ctx)
	if err != nil {
		return nil, err
	}
	idToken, err := p.exchangeIDToken(ctx, tokenEndpoint, code)
	if err != nil {
		return nil, err
	}
	claims, rawClaims, err := parseJWTClaims(idToken)
	if err != nil {
		return nil, err
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

func (p *GoogleProvider) fetchTokenEndpoint(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.discoveryURL, nil)
	if err != nil {
		return "", err
	}
	res, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer closeutil.Log(res.Body, "social: google discovery: close response body")
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("google discovery status=%d", res.StatusCode)
	}
	var body struct {
		TokenEndpoint string `json:"token_endpoint"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.TokenEndpoint == "" {
		return "", fmt.Errorf("google discovery missing token_endpoint")
	}
	return body.TokenEndpoint, nil
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

func parseJWTClaims(raw string) (map[string]any, []byte, error) {
	parts := strings.Split(raw, ".")
	if len(parts) < 2 {
		return nil, nil, fmt.Errorf("invalid jwt format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, fmt.Errorf("decode jwt payload: %w", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, nil, fmt.Errorf("parse jwt payload: %w", err)
	}
	return claims, payload, nil
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
