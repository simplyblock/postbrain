//go:build integration

package ui_test

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/oauth"
	"github.com/simplyblock/postbrain/internal/social"
	"github.com/simplyblock/postbrain/internal/testhelper"
	uiapi "github.com/simplyblock/postbrain/internal/ui"
)

type mockSocialProvider struct {
	authURL string
	info    *social.UserInfo
}

func (m *mockSocialProvider) AuthURL(state string) string {
	return m.authURL + "?state=" + url.QueryEscape(state)
}

func (m *mockSocialProvider) Exchange(_ context.Context, _ string) (*social.UserInfo, error) {
	return m.info, nil
}

func TestUISocialLoginE2E_GitHubMockProvider(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	tokenStore := auth.NewTokenStore(pool)
	stateStore := oauth.NewStateStore(pool)
	clientStore := oauth.NewClientStore(pool)
	codeStore := oauth.NewCodeStore(pool)
	issuer := oauth.NewIssuer(tokenStore)
	identityStore := social.NewIdentityStore(pool)

	var appBaseURL string
	providerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		http.Redirect(w, r, appBaseURL+"/ui/auth/github/callback?code=mock-code&state="+url.QueryEscape(state), http.StatusFound)
	}))
	defer providerSrv.Close()

	providers := map[string]social.Provider{
		"github": &mockSocialProvider{
			authURL: providerSrv.URL + "/authorize",
			info: &social.UserInfo{
				ProviderID:  "mock-gh-1",
				Email:       "mock-gh-user@example.com",
				DisplayName: "Mock GH User",
				AvatarURL:   "https://cdn.example/mock-gh-user.png",
				RawProfile:  []byte(`{"id":"mock-gh-1"}`),
			},
		},
	}

	uiCfg := config.OAuthConfig{
		BaseURL: "http://placeholder.local",
		Server: config.OAuthServerConfig{
			StateTTL: 15 * time.Minute,
			TokenTTL: 0,
		},
	}

	handler, err := uiapi.NewHandlerWithOAuth(pool, nil, uiCfg, providers, stateStore, clientStore, codeStore, issuer, identityStore)
	if err != nil {
		t.Fatalf("new ui handler: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/ui", handler)
	mux.Handle("/ui/", handler)
	appSrv := httptest.NewServer(mux)
	defer appSrv.Close()
	appBaseURL = appSrv.URL

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar}

	resp, err := client.Get(appSrv.URL + "/ui/auth/github")
	if err != nil {
		t.Fatalf("social start request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("final status = %d, want 200", resp.StatusCode)
	}

	u, _ := url.Parse(appSrv.URL + "/ui")
	cookies := jar.Cookies(u)
	found := false
	for _, c := range cookies {
		if c.Name == "pb_session" && c.Value != "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("pb_session cookie not set after social login flow")
	}
}
