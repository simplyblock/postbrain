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

type GitLabProvider struct {
	instanceURL  string
	clientID     string
	clientSecret string
	scopes       []string
	redirectURI  string
	httpClient   *http.Client
}

func NewGitLabProvider(baseURL string, cfg config.ProviderConfig) *GitLabProvider {
	instance := strings.TrimSuffix(cfg.InstanceURL, "/")
	if instance == "" {
		instance = "https://gitlab.com"
	}
	return &GitLabProvider{
		instanceURL:  instance,
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		scopes:       cfg.Scopes,
		redirectURI:  strings.TrimSuffix(baseURL, "/") + "/ui/auth/gitlab/callback",
		httpClient:   http.DefaultClient,
	}
}

func (p *GitLabProvider) AuthURL(state string) string {
	q := url.Values{}
	q.Set("client_id", p.clientID)
	q.Set("redirect_uri", p.redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", strings.Join(p.scopes, " "))
	q.Set("state", state)
	return p.instanceURL + "/oauth/authorize?" + q.Encode()
}

func (p *GitLabProvider) Exchange(ctx context.Context, code string) (*UserInfo, error) {
	accessToken, err := p.exchangeToken(ctx, code)
	if err != nil {
		return nil, err
	}
	user, raw, err := p.fetchUser(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	return &UserInfo{
		ProviderID:    strconv.FormatInt(user.ID, 10),
		Email:         user.Email,
		EmailVerified: user.Email != "",
		DisplayName:   user.Name,
		AvatarURL:     user.AvatarURL,
		RawProfile:    raw,
	}, nil
}

func (p *GitLabProvider) exchangeToken(ctx context.Context, code string) (string, error) {
	form := url.Values{}
	form.Set("client_id", p.clientID)
	form.Set("client_secret", p.clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", p.redirectURI)
	form.Set("grant_type", "authorization_code")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.instanceURL+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer closeutil.Log(res.Body, "social: gitlab token exchange: close response body")
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("gitlab token exchange status=%d", res.StatusCode)
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.AccessToken == "" {
		return "", fmt.Errorf("gitlab token exchange returned empty access_token")
	}
	return body.AccessToken, nil
}

func (p *GitLabProvider) fetchUser(ctx context.Context, accessToken string) (*struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.instanceURL+"/api/v4/user", nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	res, err := p.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer closeutil.Log(res.Body, "social: gitlab user fetch: close response body")
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("gitlab user fetch status=%d", res.StatusCode)
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
