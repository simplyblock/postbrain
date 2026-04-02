package social

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/simplyblock/postbrain/internal/closeutil"
	"github.com/simplyblock/postbrain/internal/config"
)

type GitHubProvider struct {
	clientID     string
	clientSecret string
	scopes       []string
	redirectURI  string
	httpClient   *http.Client
	authorizeURL string
	tokenURL     string
	userURL      string
	emailsURL    string
}

func NewGitHubProvider(baseURL string, cfg config.ProviderConfig) *GitHubProvider {
	return &GitHubProvider{
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		scopes:       cfg.Scopes,
		redirectURI:  strings.TrimSuffix(baseURL, "/") + "/ui/auth/github/callback",
		httpClient:   http.DefaultClient,
		authorizeURL: "https://github.com/login/oauth/authorize",
		tokenURL:     "https://github.com/login/oauth/access_token",
		userURL:      "https://api.github.com/user",
		emailsURL:    "https://api.github.com/user/emails",
	}
}

func (p *GitHubProvider) AuthURL(state string) string {
	q := url.Values{}
	q.Set("client_id", p.clientID)
	q.Set("redirect_uri", p.redirectURI)
	q.Set("scope", strings.Join(p.scopes, " "))
	q.Set("state", state)
	return p.authorizeURL + "?" + q.Encode()
}

func (p *GitHubProvider) Exchange(ctx context.Context, code string) (*UserInfo, error) {
	token, err := p.exchangeToken(ctx, code)
	if err != nil {
		return nil, err
	}
	user, raw, err := p.fetchUser(ctx, token)
	if err != nil {
		return nil, err
	}
	email := user.Email
	primaryVerified, err := p.fetchVerifiedPrimaryEmail(ctx, token)
	if err == nil && primaryVerified != "" {
		email = primaryVerified
	}
	return &UserInfo{
		ProviderID:  strconv.FormatInt(user.ID, 10),
		Email:       email,
		DisplayName: user.Name,
		AvatarURL:   user.AvatarURL,
		RawProfile:  raw,
	}, nil
}

func (p *GitHubProvider) exchangeToken(ctx context.Context, code string) (string, error) {
	form := url.Values{}
	form.Set("client_id", p.clientID)
	form.Set("client_secret", p.clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", p.redirectURI)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer closeutil.Log(res.Body, "social: github token exchange: close response body")
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("github token exchange status=%d", res.StatusCode)
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.AccessToken == "" {
		return "", fmt.Errorf("github token exchange returned empty access_token")
	}
	return body.AccessToken, nil
}

func (p *GitHubProvider) fetchUser(ctx context.Context, accessToken string) (*struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.userURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	res, err := p.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer closeutil.Log(res.Body, "social: github user fetch: close response body")
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("github user fetch status=%d", res.StatusCode)
	}
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, nil, err
	}
	var user struct {
		ID        int64  `json:"id"`
		Email     string `json:"email"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.Unmarshal(raw, &user); err != nil {
		return nil, nil, err
	}
	return &user, raw, nil
}

func (p *GitHubProvider) fetchVerifiedPrimaryEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.emailsURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	res, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer closeutil.Log(res.Body, "social: github emails fetch: close response body")
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("github emails fetch status=%d", res.StatusCode)
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(res.Body).Decode(&emails); err != nil {
		return "", err
	}
	for _, e := range emails {
		if e.Primary && e.Verified && e.Email != "" {
			return e.Email, nil
		}
	}
	return "", nil
}
