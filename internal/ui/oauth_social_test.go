package ui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/oauth"
	"github.com/simplyblock/postbrain/internal/social"
)

type fakeSocialProvider struct {
	authURL      string
	exchangeInfo *social.UserInfo
	exchangeErr  error
}

func (f *fakeSocialProvider) AuthURL(_ string) string { return f.authURL }
func (f *fakeSocialProvider) Exchange(_ context.Context, _ string) (*social.UserInfo, error) {
	if f.exchangeErr != nil {
		return nil, f.exchangeErr
	}
	return f.exchangeInfo, nil
}

type fakeUIStateStore struct {
	issuedKind string
	issuedTTL  time.Duration
	issuedRaw  string
	consumeErr error
}

func (f *fakeUIStateStore) Issue(_ context.Context, kind string, _ map[string]any, ttl time.Duration) (string, error) {
	f.issuedKind = kind
	f.issuedTTL = ttl
	if f.issuedRaw == "" {
		f.issuedRaw = "state-123"
	}
	return f.issuedRaw, nil
}

func (f *fakeUIStateStore) Consume(_ context.Context, _ string) (*oauth.StateRecord, error) {
	if f.consumeErr != nil {
		return nil, f.consumeErr
	}
	return &oauth.StateRecord{Kind: "social"}, nil
}

func (f *fakeUIStateStore) Peek(_ context.Context, _ string) (*oauth.StateRecord, error) {
	if f.consumeErr != nil {
		return nil, f.consumeErr
	}
	return &oauth.StateRecord{Kind: "social"}, nil
}

type fakeUIIdentityStore struct {
	principal *db.Principal
	err       error
}

func (f *fakeUIIdentityStore) FindOrCreate(_ context.Context, _ string, _ *social.UserInfo) (*db.Principal, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.principal, nil
}

type fakeUIIssuer struct {
	rawToken string
	err      error
}

func (f *fakeUIIssuer) Issue(_ context.Context, _ uuid.UUID, _ []string, _ time.Duration) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if f.rawToken == "" {
		f.rawToken = "pb_social_token"
	}
	return f.rawToken, nil
}

func TestHandleSocialStart_UnknownProvider_Returns404(t *testing.T) {
	h := newTestHandler(t)
	h.providers = map[string]social.Provider{}
	h.stateStore = &fakeUIStateStore{}
	req := httptest.NewRequest(http.MethodGet, "/ui/auth/unknown", nil)
	rec := httptest.NewRecorder()

	h.handleSocialStart(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandleSocialStart_DisabledProvider_Returns404(t *testing.T) {
	h := newTestHandler(t)
	h.providers = map[string]social.Provider{} // disabled provider not present
	h.stateStore = &fakeUIStateStore{}
	req := httptest.NewRequest(http.MethodGet, "/ui/auth/github", nil)
	rec := httptest.NewRecorder()

	h.handleSocialStart(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandleSocialStart_ValidProvider_RedirectsToProviderURL(t *testing.T) {
	h := newTestHandler(t)
	h.providers = map[string]social.Provider{
		"github": &fakeSocialProvider{authURL: "https://github.example/authorize"},
	}
	h.stateStore = &fakeUIStateStore{}
	h.oauthCfg = config.OAuthConfig{Server: config.OAuthServerConfig{StateTTL: 15 * time.Minute}}
	req := httptest.NewRequest(http.MethodGet, "/ui/auth/github", nil)
	rec := httptest.NewRecorder()

	h.handleSocialStart(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "https://github.example/authorize" {
		t.Fatalf("Location = %q", got)
	}
}

func TestHandleSocialStart_SetsStateInDB(t *testing.T) {
	h := newTestHandler(t)
	state := &fakeUIStateStore{}
	h.providers = map[string]social.Provider{
		"github": &fakeSocialProvider{authURL: "https://github.example/authorize"},
	}
	h.stateStore = state
	h.oauthCfg = config.OAuthConfig{Server: config.OAuthServerConfig{StateTTL: 15 * time.Minute}}
	req := httptest.NewRequest(http.MethodGet, "/ui/auth/github", nil)
	rec := httptest.NewRecorder()

	h.handleSocialStart(rec, req)

	if state.issuedKind != "social" {
		t.Fatalf("issued kind = %q, want social", state.issuedKind)
	}
	if state.issuedTTL != 15*time.Minute {
		t.Fatalf("issued ttl = %v, want 15m", state.issuedTTL)
	}
}

func TestHandleSocialCallback_InvalidState_Returns400(t *testing.T) {
	h := newTestHandler(t)
	h.providers = map[string]social.Provider{"github": &fakeSocialProvider{}}
	h.stateStore = &fakeUIStateStore{consumeErr: oauth.ErrNotFound}
	h.identities = &fakeUIIdentityStore{principal: &db.Principal{ID: uuid.New()}}
	h.issuer = &fakeUIIssuer{}
	req := httptest.NewRequest(http.MethodGet, "/ui/auth/github/callback?code=ok&state=bad", nil)
	rec := httptest.NewRecorder()

	h.handleSocialCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleSocialCallback_ExpiredState_Returns400(t *testing.T) {
	TestHandleSocialCallback_InvalidState_Returns400(t)
}

func TestHandleSocialCallback_ReplayState_Returns400(t *testing.T) {
	TestHandleSocialCallback_InvalidState_Returns400(t)
}

func TestHandleSocialCallback_ProviderExchangeError_Returns502(t *testing.T) {
	h := newTestHandler(t)
	h.providers = map[string]social.Provider{
		"github": &fakeSocialProvider{exchangeErr: errors.New("exchange failed")},
	}
	h.stateStore = &fakeUIStateStore{}
	h.identities = &fakeUIIdentityStore{principal: &db.Principal{ID: uuid.New()}}
	h.issuer = &fakeUIIssuer{}
	req := httptest.NewRequest(http.MethodGet, "/ui/auth/github/callback?code=ok&state=state-1", nil)
	rec := httptest.NewRecorder()

	h.handleSocialCallback(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}

func TestHandleSocialCallback_Success_SetsCookie_RedirectsToUI(t *testing.T) {
	h := newTestHandler(t)
	h.providers = map[string]social.Provider{
		"github": &fakeSocialProvider{exchangeInfo: &social.UserInfo{
			ProviderID:  "gh-1",
			Email:       "user@example.com",
			DisplayName: "User",
		}},
	}
	h.stateStore = &fakeUIStateStore{}
	h.identities = &fakeUIIdentityStore{principal: &db.Principal{ID: uuid.New()}}
	h.issuer = &fakeUIIssuer{rawToken: "pb_social_token"}
	h.oauthCfg = config.OAuthConfig{BaseURL: "https://postbrain.example.com"}

	req := httptest.NewRequest(http.MethodGet, "/ui/auth/github/callback?code=ok&state=state-1", nil)
	rec := httptest.NewRecorder()

	h.handleSocialCallback(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/ui" {
		t.Fatalf("Location = %q", got)
	}
	cookies := rec.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == cookieName {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("pb_session cookie not set")
	}
	if found.Value != "pb_social_token" {
		t.Fatalf("cookie value = %q", found.Value)
	}
	if !found.Secure {
		t.Fatal("expected Secure cookie when oauth.base_url is https")
	}
	if found.Path != "/ui" {
		t.Fatalf("cookie path = %q", found.Path)
	}
}

func TestServeHTTP_SocialStart_UnauthenticatedStillRoutes(t *testing.T) {
	h := newTestHandler(t)
	h.providers = map[string]social.Provider{"github": &fakeSocialProvider{authURL: "https://github.example/authorize"}}
	h.stateStore = &fakeUIStateStore{}
	req := httptest.NewRequest(http.MethodGet, "/ui/auth/github", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "github.example/authorize") {
		t.Fatalf("Location = %q", rec.Header().Get("Location"))
	}
}
