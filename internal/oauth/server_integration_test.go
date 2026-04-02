//go:build integration

package oauth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	mcpapi "github.com/simplyblock/postbrain/internal/api/mcp"
	restapi "github.com/simplyblock/postbrain/internal/api/rest"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/oauth"
	"github.com/simplyblock/postbrain/internal/social"
	"github.com/simplyblock/postbrain/internal/testhelper"
	uiapi "github.com/simplyblock/postbrain/internal/ui"
)

func TestOAuthServer_AuthorizationCodePKCERoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()

	cfg := &config.Config{
		OAuth: config.OAuthConfig{
			BaseURL: "http://example.test",
			Server: config.OAuthServerConfig{
				AuthCodeTTL:         10 * time.Minute,
				StateTTL:            15 * time.Minute,
				TokenTTL:            0,
				DynamicRegistration: true,
			},
		},
	}

	user := testhelper.CreateTestPrincipal(t, pool, "user", "oauth-int-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "oauth-int-project", nil, user.ID)

	// UI session cookie token.
	rawSession, hashSession, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	_, err = db.CreateToken(ctx, pool, user.ID, hashSession, "ui-session", nil, []string{
		oauth.ScopeMemoriesRead, oauth.ScopeMemoriesWrite,
		oauth.ScopeKnowledgeRead, oauth.ScopeKnowledgeWrite,
		oauth.ScopeSkillsRead, oauth.ScopeSkillsWrite, oauth.ScopeAdmin,
	}, nil)
	if err != nil {
		t.Fatalf("create session token: %v", err)
	}

	mux := http.NewServeMux()
	tokenStore := auth.NewTokenStore(pool)
	stateStore := oauth.NewStateStore(pool)
	clientStore := oauth.NewClientStore(pool)
	codeStore := oauth.NewCodeStore(pool)
	issuer := oauth.NewIssuer(tokenStore)
	identityStore := social.NewIdentityStore(pool)
	providers := social.NewRegistry(cfg.OAuth)
	oauthServer := oauth.NewServer(clientStore, codeStore, stateStore, issuer, tokenStore, cfg.OAuth)

	mux.HandleFunc("GET /.well-known/oauth-authorization-server", oauthServer.HandleMetadata)
	mux.HandleFunc("GET /oauth/authorize", oauthServer.HandleAuthorize)
	mux.HandleFunc("POST /oauth/token", oauthServer.HandleToken)
	mux.HandleFunc("POST /oauth/register", oauthServer.HandleRegister)
	mux.HandleFunc("POST /oauth/revoke", oauthServer.HandleRevoke)

	restSrv := restapi.NewRouter(pool, svc, cfg)
	mux.Handle("/", restSrv.Handler())
	mcpSrv := mcpapi.NewServer(pool, svc, cfg)
	mux.Handle("/mcp", mcpSrv.Handler())
	mux.Handle("/mcp/", mcpSrv.Handler())

	uiHandler, err := uiapi.NewHandlerWithOAuth(pool, svc, cfg.OAuth, providers, stateStore, clientStore, codeStore, issuer, identityStore)
	if err != nil {
		t.Fatalf("new ui handler: %v", err)
	}
	mux.Handle("/ui", uiHandler)
	mux.Handle("/ui/", uiHandler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Dynamic registration (public client).
	clientID := registerOAuthClient(t, srv.URL, map[string]any{
		"client_name":                "OAuth Int Public",
		"redirect_uris":              []string{"http://localhost:8765/callback"},
		"grant_types":                []string{"authorization_code"},
		"token_endpoint_auth_method": "none",
	})

	verifier := "integration-verifier-public"
	challenge := oauth.GenerateChallenge(verifier)
	code := authorizeAndApprove(t, srv.URL, clientID, "http://localhost:8765/callback", "memories:read", "orig-state-1", challenge, rawSession)

	accessToken := exchangeToken(t, srv.URL, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {"http://localhost:8765/callback"},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	}, http.StatusOK, "access_token")

	// Issued token works on scope-locked endpoint.
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/memories/recall?q=test&scope=project:"+scope.ExternalID, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("recall request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("recall status = %d, want 200. body=%s", resp.StatusCode, string(body))
	}

	// Replay same code.
	exchangeToken(t, srv.URL, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {"http://localhost:8765/callback"},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	}, http.StatusBadRequest, "invalid_grant")

	// PKCE mismatch.
	codeForPKCEMismatch := authorizeAndApprove(t, srv.URL, clientID, "http://localhost:8765/callback", "memories:read", "orig-state-2", challenge, rawSession)
	exchangeToken(t, srv.URL, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {codeForPKCEMismatch},
		"redirect_uri":  {"http://localhost:8765/callback"},
		"client_id":     {clientID},
		"code_verifier": {"wrong-verifier"},
	}, http.StatusBadRequest, "invalid_grant")

	// Redirect URI mismatch.
	codeForRedirectMismatch := authorizeAndApprove(t, srv.URL, clientID, "http://localhost:8765/callback", "memories:read", "orig-state-3", challenge, rawSession)
	exchangeToken(t, srv.URL, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {codeForRedirectMismatch},
		"redirect_uri":  {"http://localhost:9999/callback"},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	}, http.StatusBadRequest, "invalid_grant")

	// Confidential client with bad secret.
	confidentialRegBody := map[string]any{
		"client_name":                "OAuth Int Confidential",
		"redirect_uris":              []string{"http://localhost:8765/callback"},
		"grant_types":                []string{"authorization_code"},
		"token_endpoint_auth_method": "client_secret_post",
	}
	confResp := registerOAuthClientRaw(t, srv.URL, confidentialRegBody)
	confClientID, _ := confResp["client_id"].(string)
	confChallenge := oauth.GenerateChallenge("integration-verifier-conf")
	confCode := authorizeAndApprove(t, srv.URL, confClientID, "http://localhost:8765/callback", "memories:read", "orig-state-4", confChallenge, rawSession)
	exchangeToken(t, srv.URL, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {confCode},
		"redirect_uri":  {"http://localhost:8765/callback"},
		"client_id":     {confClientID},
		"code_verifier": {"integration-verifier-conf"},
		"client_secret": {"wrong-secret"},
	}, http.StatusUnauthorized, "invalid_client")
}

func TestOAuthServer_DynamicRegistrationDisabled(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := config.OAuthConfig{
		BaseURL: "http://example.test",
		Server: config.OAuthServerConfig{
			AuthCodeTTL:         10 * time.Minute,
			StateTTL:            15 * time.Minute,
			TokenTTL:            0,
			DynamicRegistration: false,
		},
	}

	tokenStore := auth.NewTokenStore(pool)
	stateStore := oauth.NewStateStore(pool)
	clientStore := oauth.NewClientStore(pool)
	codeStore := oauth.NewCodeStore(pool)
	issuer := oauth.NewIssuer(tokenStore)
	oauthServer := oauth.NewServer(clientStore, codeStore, stateStore, issuer, tokenStore, cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth/register", oauthServer.HandleRegister)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{
		"client_name":   "disabled-reg",
		"redirect_uris": []string{"http://localhost:8765/callback"},
	})
	resp, err := http.Post(srv.URL+"/oauth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("register request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}

	_ = svc
}

func registerOAuthClient(t *testing.T, baseURL string, body map[string]any) string {
	t.Helper()
	respBody := registerOAuthClientRaw(t, baseURL, body)
	clientID, _ := respBody["client_id"].(string)
	if clientID == "" {
		t.Fatalf("missing client_id in response: %v", respBody)
	}
	return clientID
}

func registerOAuthClientRaw(t *testing.T, baseURL string, body map[string]any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(baseURL+"/oauth/register", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("register request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("register status = %d, want 201. body=%s", resp.StatusCode, string(raw))
	}
	out := map[string]any{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	return out
}

func authorizeAndApprove(t *testing.T, baseURL, clientID, redirectURI, scope, originalState, challenge, sessionToken string) string {
	t.Helper()

	noRedirect := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	authURL := baseURL + "/oauth/authorize?response_type=code" +
		"&client_id=" + url.QueryEscape(clientID) +
		"&redirect_uri=" + url.QueryEscape(redirectURI) +
		"&scope=" + url.QueryEscape(scope) +
		"&state=" + url.QueryEscape(originalState) +
		"&code_challenge=" + url.QueryEscape(challenge) +
		"&code_challenge_method=S256"
	resp, err := noRedirect.Get(authURL)
	if err != nil {
		t.Fatalf("authorize request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("authorize status = %d, want 302. body=%s", resp.StatusCode, string(body))
	}
	consentLocation := resp.Header.Get("Location")
	if !strings.HasPrefix(consentLocation, "/ui/oauth/authorize?state=") {
		t.Fatalf("unexpected authorize redirect location: %q", consentLocation)
	}
	consentURL := baseURL + consentLocation
	consentState := strings.TrimPrefix(consentLocation, "/ui/oauth/authorize?state=")

	consentGetReq, _ := http.NewRequest(http.MethodGet, consentURL, nil)
	consentGetReq.AddCookie(&http.Cookie{Name: "pb_session", Value: sessionToken, Path: "/ui"})
	consentGetResp, err := noRedirect.Do(consentGetReq)
	if err != nil {
		t.Fatalf("consent GET request: %v", err)
	}
	defer consentGetResp.Body.Close()
	if consentGetResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(consentGetResp.Body)
		t.Fatalf("consent GET status = %d, want 200. body=%s", consentGetResp.StatusCode, string(body))
	}

	form := url.Values{}
	form.Set("state", consentState)
	form.Set("action", "approve")
	consentPostReq, _ := http.NewRequest(http.MethodPost, baseURL+"/ui/oauth/authorize", strings.NewReader(form.Encode()))
	consentPostReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	consentPostReq.AddCookie(&http.Cookie{Name: "pb_session", Value: sessionToken, Path: "/ui"})
	consentPostResp, err := noRedirect.Do(consentPostReq)
	if err != nil {
		t.Fatalf("consent POST request: %v", err)
	}
	defer consentPostResp.Body.Close()
	if consentPostResp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(consentPostResp.Body)
		t.Fatalf("consent POST status = %d, want 302. body=%s", consentPostResp.StatusCode, string(body))
	}
	redirected := consentPostResp.Header.Get("Location")
	parsed, err := url.Parse(redirected)
	if err != nil {
		t.Fatalf("parse consent redirect: %v", err)
	}
	code := parsed.Query().Get("code")
	if code == "" {
		t.Fatalf("missing code in consent redirect: %q", redirected)
	}
	if gotState := parsed.Query().Get("state"); gotState != originalState {
		t.Fatalf("returned state = %q, want %q", gotState, originalState)
	}
	return code
}

func exchangeToken(t *testing.T, baseURL string, form url.Values, wantStatus int, wantTokenOrError string) string {
	t.Helper()
	resp, err := http.Post(baseURL+"/oauth/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("token exchange request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("token status = %d, want %d. body=%s", resp.StatusCode, wantStatus, string(body))
	}
	m := map[string]any{}
	_ = json.Unmarshal(body, &m)
	if wantStatus == http.StatusOK {
		token, _ := m["access_token"].(string)
		if token == "" {
			t.Fatalf("missing access_token in body=%s", string(body))
		}
		return token
	}
	if gotErr, _ := m["error"].(string); gotErr != wantTokenOrError {
		t.Fatalf("error=%q, want %q, body=%s", gotErr, wantTokenOrError, string(body))
	}
	return ""
}
