package oauth

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
)

type fakeServerClientStore struct {
	byClientID map[string]*OAuthClient
	lastRegReq RegisterRequest
}

func (f *fakeServerClientStore) LookupByClientID(_ context.Context, clientID string) (*OAuthClient, error) {
	return f.byClientID[clientID], nil
}

func (f *fakeServerClientStore) ValidateRedirectURI(client *OAuthClient, redirectURI string) error {
	for _, u := range client.RedirectURIs {
		if u == redirectURI {
			return nil
		}
	}
	return ErrInvalidRedirectURI
}

func (f *fakeServerClientStore) Register(_ context.Context, req RegisterRequest) (*OAuthClient, string, error) {
	f.lastRegReq = req
	c := &OAuthClient{
		ID:           uuid.New(),
		ClientID:     "pb_client_registered",
		Name:         req.Name,
		RedirectURIs: req.RedirectURIs,
		GrantTypes:   req.GrantTypes,
		Scopes:       req.Scopes,
		IsPublic:     req.IsPublic,
		Meta:         req.Meta,
	}
	if req.IsPublic {
		return c, "", nil
	}
	hash := hashSHA256Hex("raw-secret")
	c.ClientSecretHash = &hash
	return c, "raw-secret", nil
}

type fakeServerCodeStore struct {
	code        *AuthCode
	consumeErr  error
	verifyErr   error
	consumedRaw []string
}

func (f *fakeServerCodeStore) Consume(_ context.Context, rawCode string) (*AuthCode, error) {
	f.consumedRaw = append(f.consumedRaw, rawCode)
	if f.consumeErr != nil {
		return nil, f.consumeErr
	}
	return f.code, nil
}

func (f *fakeServerCodeStore) VerifyPKCE(_ *AuthCode, _ string) error {
	return f.verifyErr
}

type fakeServerStateStore struct {
	lastKind    string
	lastPayload map[string]any
	lastTTL     time.Duration
	rawState    string
}

func (f *fakeServerStateStore) Issue(_ context.Context, kind string, payload map[string]any, ttl time.Duration) (string, error) {
	f.lastKind = kind
	f.lastPayload = payload
	f.lastTTL = ttl
	if f.rawState == "" {
		f.rawState = "consent-state"
	}
	return f.rawState, nil
}

type fakeServerTokenStore struct {
	tokenByHash map[string]*db.Token
	revoked     []uuid.UUID
}

func (f *fakeServerTokenStore) Lookup(_ context.Context, hash string) (*db.Token, error) {
	return f.tokenByHash[hash], nil
}

func (f *fakeServerTokenStore) Revoke(_ context.Context, tokenID uuid.UUID) error {
	f.revoked = append(f.revoked, tokenID)
	return nil
}

type fakeServerIssuer struct {
	lastPrincipal uuid.UUID
	lastScopes    []string
	lastTTL       time.Duration
	rawToken      string
}

func (f *fakeServerIssuer) Issue(_ context.Context, principalID uuid.UUID, scopes []string, ttl time.Duration) (string, error) {
	f.lastPrincipal = principalID
	f.lastScopes = scopes
	f.lastTTL = ttl
	if f.rawToken == "" {
		f.rawToken = "pb_token_test"
	}
	return f.rawToken, nil
}

func newTestServer() *Server {
	clients := &fakeServerClientStore{
		byClientID: map[string]*OAuthClient{
			"pb_client_public": {
				ID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
				ClientID:     "pb_client_public",
				Name:         "Public",
				RedirectURIs: []string{"http://localhost:8765/callback"},
				IsPublic:     true,
			},
			"pb_client_confidential": {
				ID:               uuid.MustParse("22222222-2222-2222-2222-222222222222"),
				ClientID:         "pb_client_confidential",
				Name:             "Confidential",
				RedirectURIs:     []string{"http://localhost:8765/callback"},
				IsPublic:         false,
				ClientSecretHash: strPtr(hashSHA256Hex("correct-secret")),
			},
		},
	}
	codes := &fakeServerCodeStore{
		code: &AuthCode{
			ID:            uuid.New(),
			CodeHash:      "hash",
			ClientID:      uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			PrincipalID:   uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
			RedirectURI:   "http://localhost:8765/callback",
			Scopes:        []string{ScopeMemoriesRead},
			CodeChallenge: GenerateChallenge("verifier"),
			ExpiresAt:     time.Now().Add(10 * time.Minute),
		},
	}
	states := &fakeServerStateStore{}
	issuer := &fakeServerIssuer{rawToken: "pb_test_token"}
	tokens := &fakeServerTokenStore{tokenByHash: map[string]*db.Token{}}
	cfg := config.OAuthConfig{
		BaseURL: "https://postbrain.example.com",
		Server: config.OAuthServerConfig{
			AuthCodeTTL:         10 * time.Minute,
			StateTTL:            15 * time.Minute,
			TokenTTL:            0,
			DynamicRegistration: true,
		},
	}
	return NewServer(clients, codes, states, issuer, tokens, cfg)
}

func TestHandleMetadata_Returns200_WithRequiredFields(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	rec := httptest.NewRecorder()

	srv.HandleMetadata(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, key := range []string{"issuer", "authorization_endpoint", "token_endpoint", "registration_endpoint"} {
		if !strings.Contains(body, key) {
			t.Fatalf("response missing key %q: %s", key, body)
		}
	}
}

func TestHandleAuthorize_MissingClientID_Returns400(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?response_type=code&redirect_uri=http://localhost:8765/callback&scope=memories:read&state=s", nil)
	rec := httptest.NewRecorder()
	srv.HandleAuthorize(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleAuthorize_MissingState_Returns400(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?response_type=code&client_id=pb_client_public&redirect_uri=http://localhost:8765/callback&scope=memories:read", nil)
	rec := httptest.NewRecorder()
	srv.HandleAuthorize(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleAuthorize_InvalidResponseType_Returns400(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?response_type=token&client_id=pb_client_public&redirect_uri=http://localhost:8765/callback&scope=memories:read&state=s", nil)
	rec := httptest.NewRecorder()
	srv.HandleAuthorize(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleAuthorize_UnknownClientID_Returns400(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?response_type=code&client_id=unknown&redirect_uri=http://localhost:8765/callback&scope=memories:read&state=s", nil)
	rec := httptest.NewRecorder()
	srv.HandleAuthorize(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleAuthorize_BadRedirectURI_Returns400(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?response_type=code&client_id=pb_client_public&redirect_uri=http://localhost:9999/callback&scope=memories:read&state=s", nil)
	rec := httptest.NewRecorder()
	srv.HandleAuthorize(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleAuthorize_UnknownScope_Returns400(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?response_type=code&client_id=pb_client_public&redirect_uri=http://localhost:8765/callback&scope=unknown:scope&state=s", nil)
	rec := httptest.NewRecorder()
	srv.HandleAuthorize(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleAuthorize_MissingCodeChallenge_PublicClient_Returns400(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?response_type=code&client_id=pb_client_public&redirect_uri=http://localhost:8765/callback&scope=memories:read&state=s", nil)
	rec := httptest.NewRecorder()
	srv.HandleAuthorize(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleAuthorize_PlainCodeChallengeMethod_Returns400(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?response_type=code&client_id=pb_client_public&redirect_uri=http://localhost:8765/callback&scope=memories:read&state=s&code_challenge=x&code_challenge_method=plain", nil)
	rec := httptest.NewRecorder()
	srv.HandleAuthorize(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleAuthorize_ValidRequest_RedirectsToConsentUI(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?response_type=code&client_id=pb_client_public&redirect_uri=http://localhost:8765/callback&scope=memories:read&state=orig-state&code_challenge=abc&code_challenge_method=S256", nil)
	rec := httptest.NewRecorder()
	srv.HandleAuthorize(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "/ui/oauth/authorize?state=") {
		t.Fatalf("location = %q, want consent redirect", location)
	}
}

func TestHandleAuthorize_StoresValidatedPayloadInState(t *testing.T) {
	srv := newTestServer()
	states := srv.states.(*fakeServerStateStore)
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?response_type=code&client_id=pb_client_public&redirect_uri=http://localhost:8765/callback&scope=memories:read&state=orig-state&code_challenge=abc&code_challenge_method=S256", nil)
	rec := httptest.NewRecorder()

	srv.HandleAuthorize(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if states.lastKind != "mcp_consent" {
		t.Fatalf("state kind = %q, want mcp_consent", states.lastKind)
	}
	if states.lastPayload["client_id"] != "pb_client_public" {
		t.Fatalf("state payload client_id = %v", states.lastPayload["client_id"])
	}
	if states.lastPayload["state"] != "orig-state" {
		t.Fatalf("state payload state = %v", states.lastPayload["state"])
	}
}

func TestHandleToken_MissingCode_Returns400(t *testing.T) {
	srv := newTestServer()
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {"pb_client_public"}, "redirect_uri": {"http://localhost:8765/callback"}}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.HandleToken(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleToken_MissingGrantType_Returns400(t *testing.T) {
	srv := newTestServer()
	form := url.Values{"code": {"raw-code"}, "client_id": {"pb_client_public"}, "redirect_uri": {"http://localhost:8765/callback"}}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.HandleToken(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleToken_InvalidCode_Returns400(t *testing.T) {
	srv := newTestServer()
	codes := srv.codes.(*fakeServerCodeStore)
	codes.consumeErr = ErrCodeNotFound

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"raw-code"},
		"client_id":     {"pb_client_public"},
		"redirect_uri":  {"http://localhost:8765/callback"},
		"code_verifier": {"verifier"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.HandleToken(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleToken_RedirectURIMismatch_Returns400(t *testing.T) {
	srv := newTestServer()
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"raw-code"},
		"client_id":     {"pb_client_public"},
		"redirect_uri":  {"http://localhost:9999/callback"},
		"code_verifier": {"verifier"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.HandleToken(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleToken_ClientIDMismatch_Returns400(t *testing.T) {
	srv := newTestServer()
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"raw-code"},
		"client_id":     {"pb_client_confidential"},
		"redirect_uri":  {"http://localhost:8765/callback"},
		"code_verifier": {"verifier"},
		"client_secret": {"correct-secret"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.HandleToken(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleToken_PKCEMismatch_Returns400(t *testing.T) {
	srv := newTestServer()
	codes := srv.codes.(*fakeServerCodeStore)
	codes.verifyErr = ErrPKCEMismatch
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"raw-code"},
		"client_id":     {"pb_client_public"},
		"redirect_uri":  {"http://localhost:8765/callback"},
		"code_verifier": {"wrong"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.HandleToken(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleToken_ConfidentialClient_MissingSecret_Returns401(t *testing.T) {
	srv := newTestServer()
	// Code belongs to confidential client for this test.
	srv.codes.(*fakeServerCodeStore).code.ClientID = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"raw-code"},
		"client_id":     {"pb_client_confidential"},
		"redirect_uri":  {"http://localhost:8765/callback"},
		"code_verifier": {"verifier"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.HandleToken(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHandleToken_ConfidentialClient_WrongSecret_Returns401(t *testing.T) {
	srv := newTestServer()
	srv.codes.(*fakeServerCodeStore).code.ClientID = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"raw-code"},
		"client_id":     {"pb_client_confidential"},
		"redirect_uri":  {"http://localhost:8765/callback"},
		"code_verifier": {"verifier"},
		"client_secret": {"wrong-secret"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.HandleToken(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHandleToken_ConfidentialClient_ValidSecret_NoVerifier_Returns200(t *testing.T) {
	srv := newTestServer()
	codes := srv.codes.(*fakeServerCodeStore)
	codes.code.ClientID = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	codes.code.CodeChallenge = ""

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"raw-code"},
		"client_id":     {"pb_client_confidential"},
		"redirect_uri":  {"http://localhost:8765/callback"},
		"client_secret": {"correct-secret"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.HandleToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "access_token") {
		t.Fatalf("expected access_token in response body, got %s", rec.Body.String())
	}
}

func TestHandleToken_ValidRequest_Returns200_WithToken(t *testing.T) {
	srv := newTestServer()
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"raw-code"},
		"client_id":     {"pb_client_public"},
		"redirect_uri":  {"http://localhost:8765/callback"},
		"code_verifier": {"verifier"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.HandleToken(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "access_token") {
		t.Fatalf("expected access_token in response, got: %s", rec.Body.String())
	}
}

func TestHandleToken_ReplayCode_Returns400(t *testing.T) {
	srv := newTestServer()
	codes := srv.codes.(*fakeServerCodeStore)
	codes.consumeErr = ErrCodeUsed
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"raw-code"},
		"client_id":     {"pb_client_public"},
		"redirect_uri":  {"http://localhost:8765/callback"},
		"code_verifier": {"verifier"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.HandleToken(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleRegister_ValidPublicClient_Returns201(t *testing.T) {
	srv := newTestServer()
	body := []byte(`{"client_name":"Claude Desktop","redirect_uris":["http://localhost:8765/callback"],"grant_types":["authorization_code"],"token_endpoint_auth_method":"none"}`)
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	srv.HandleRegister(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
}

func TestHandleRegister_ConfidentialClient_ReturnsClientSecretOnce(t *testing.T) {
	srv := newTestServer()
	body := []byte(`{"client_name":"VS Code","redirect_uris":["http://localhost:8765/callback"],"grant_types":["authorization_code"],"token_endpoint_auth_method":"client_secret_post"}`)
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	srv.HandleRegister(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "client_secret") {
		t.Fatalf("expected client_secret in response, got %s", rec.Body.String())
	}
}

func TestHandleRegister_RateLimitExceeded_Returns429(t *testing.T) {
	srv := newTestServer()
	srv.limiter.now = func() time.Time { return time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC) }
	body := []byte(`{"client_name":"Claude Desktop","redirect_uris":["http://localhost:8765/callback"],"grant_types":["authorization_code"],"token_endpoint_auth_method":"none"}`)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodPost, "/oauth/register", bytes.NewReader(body))
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()
		srv.HandleRegister(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("request %d status = %d, want 201", i+1, rec.Code)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	srv.HandleRegister(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
}

func TestHandleRegister_DynamicRegistrationDisabled_Returns404(t *testing.T) {
	srv := newTestServer()
	srv.cfg.Server.DynamicRegistration = false
	body := []byte(`{"client_name":"Claude Desktop","redirect_uris":["http://localhost:8765/callback"]}`)
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	srv.HandleRegister(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// TestHandleRevoke_NoAuth_Returns401 is a regression test for the missing-auth
// vulnerability: an unauthenticated request must be rejected, not processed.
func TestHandleRevoke_NoAuth_Returns401(t *testing.T) {
	srv := newTestServer()
	rawToken := "pb_victim_token"
	hash := hashSHA256Hex(rawToken)
	srv.tokens.(*fakeServerTokenStore).tokenByHash[hash] = &db.Token{ID: uuid.New()}

	// No Authorization header — simulates the unauthenticated SSRF/DoS attack.
	form := url.Values{"token": {rawToken}}
	req := httptest.NewRequest(http.MethodPost, "/oauth/revoke", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.HandleRevoke(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (unauthenticated revoke must be rejected)", rec.Code)
	}
	if len(srv.tokens.(*fakeServerTokenStore).revoked) != 0 {
		t.Fatalf("revoke calls = %d, want 0 (token must NOT be revoked without auth)", len(srv.tokens.(*fakeServerTokenStore).revoked))
	}
}

// TestHandleRevoke_InvalidBearerToken_Returns401 verifies that a request
// carrying an expired or already-revoked bearer token is rejected.
func TestHandleRevoke_InvalidBearerToken_Returns401(t *testing.T) {
	srv := newTestServer()
	// Bearer token is not in the store (simulates revoked/expired token).
	req := httptest.NewRequest(http.MethodPost, "/oauth/revoke", strings.NewReader("token=pb_any"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer pb_invalid_bearer")
	rec := httptest.NewRecorder()
	srv.HandleRevoke(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (invalid bearer must be rejected)", rec.Code)
	}
}

// TestHandleRevoke_CrossTokenRevocation_NotRevoked verifies that a valid
// bearer holder cannot revoke a different token (cross-token DoS prevention).
func TestHandleRevoke_CrossTokenRevocation_NotRevoked(t *testing.T) {
	srv := newTestServer()
	bearerRaw := "pb_attacker_token"
	victimRaw := "pb_victim_token"
	for _, pair := range []struct{ raw, id string }{{bearerRaw, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}, {victimRaw, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"}} {
		id := uuid.MustParse(pair.id)
		srv.tokens.(*fakeServerTokenStore).tokenByHash[hashSHA256Hex(pair.raw)] = &db.Token{ID: id}
	}

	// Authenticate as attacker, but try to revoke the victim's token.
	form := url.Values{"token": {victimRaw}}
	req := httptest.NewRequest(http.MethodPost, "/oauth/revoke", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+bearerRaw)
	rec := httptest.NewRecorder()
	srv.HandleRevoke(rec, req)

	// Must return 200 (RFC 7009 §2.2 — server responds 200 for tokens it won't revoke)
	// but must NOT actually revoke the victim's token.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(srv.tokens.(*fakeServerTokenStore).revoked) != 0 {
		t.Fatalf("revoke calls = %d, want 0 (victim token must NOT be revoked)", len(srv.tokens.(*fakeServerTokenStore).revoked))
	}
}

// TestHandleRevoke_ValidToken_Returns200 verifies that a token holder can
// revoke their own active token (self-revocation).
func TestHandleRevoke_ValidToken_Returns200(t *testing.T) {
	srv := newTestServer()
	rawToken := "pb_revoke_me"
	hash := hashSHA256Hex(rawToken)
	tokenID := uuid.New()
	srv.tokens.(*fakeServerTokenStore).tokenByHash[hash] = &db.Token{ID: tokenID}

	form := url.Values{"token": {rawToken}}
	req := httptest.NewRequest(http.MethodPost, "/oauth/revoke", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+rawToken) // authenticate with the same token
	rec := httptest.NewRecorder()
	srv.HandleRevoke(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(srv.tokens.(*fakeServerTokenStore).revoked) != 1 {
		t.Fatalf("revoke calls = %d, want 1", len(srv.tokens.(*fakeServerTokenStore).revoked))
	}
}

// TestHandleRevoke_BodyTokenMissing_Returns200 verifies that omitting the
// body token parameter while providing valid auth returns 200 with no action.
func TestHandleRevoke_BodyTokenMissing_Returns200(t *testing.T) {
	srv := newTestServer()
	rawToken := "pb_auth_no_body_token"
	hash := hashSHA256Hex(rawToken)
	srv.tokens.(*fakeServerTokenStore).tokenByHash[hash] = &db.Token{ID: uuid.New()}

	req := httptest.NewRequest(http.MethodPost, "/oauth/revoke", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+rawToken)
	rec := httptest.NewRecorder()
	srv.HandleRevoke(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(srv.tokens.(*fakeServerTokenStore).revoked) != 0 {
		t.Fatalf("revoke calls = %d, want 0", len(srv.tokens.(*fakeServerTokenStore).revoked))
	}
}

func strPtr(s string) *string { return &s }

// TestHandleRevoke_BearerScheme_CaseInsensitive verifies that RFC 9110 §11.1
// case-insensitive scheme matching accepts "bearer" and "BEARER" in addition to
// the canonical "Bearer" form.
func TestHandleRevoke_BearerScheme_CaseInsensitive(t *testing.T) {
	for _, scheme := range []string{"Bearer", "bearer", "BEARER", "bEaReR"} {
		scheme := scheme
		t.Run(scheme, func(t *testing.T) {
			srv := newTestServer()
			rawToken := "pb_case_token"
			hash := hashSHA256Hex(rawToken)
			srv.tokens.(*fakeServerTokenStore).tokenByHash[hash] = &db.Token{ID: uuid.New()}

			form := url.Values{"token": {rawToken}}
			req := httptest.NewRequest(http.MethodPost, "/oauth/revoke", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Authorization", scheme+" "+rawToken)
			rec := httptest.NewRecorder()
			srv.HandleRevoke(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("scheme %q: status = %d, want 200", scheme, rec.Code)
			}
			if len(srv.tokens.(*fakeServerTokenStore).revoked) != 1 {
				t.Errorf("scheme %q: revoke calls = %d, want 1", scheme, len(srv.tokens.(*fakeServerTokenStore).revoked))
			}
		})
	}
}

// TestHandleRevoke_WhitespaceAroundToken_Accepted verifies that extra whitespace
// around the bearer token value is trimmed (RFC 9110 §5.6.2).
func TestHandleRevoke_WhitespaceAroundToken_Accepted(t *testing.T) {
	srv := newTestServer()
	rawToken := "pb_ws_token"
	hash := hashSHA256Hex(rawToken)
	srv.tokens.(*fakeServerTokenStore).tokenByHash[hash] = &db.Token{ID: uuid.New()}

	form := url.Values{"token": {rawToken}}
	req := httptest.NewRequest(http.MethodPost, "/oauth/revoke", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer  "+rawToken+"  ") // extra spaces
	rec := httptest.NewRecorder()
	srv.HandleRevoke(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (whitespace must be trimmed)", rec.Code)
	}
	if len(srv.tokens.(*fakeServerTokenStore).revoked) != 1 {
		t.Fatalf("revoke calls = %d, want 1", len(srv.tokens.(*fakeServerTokenStore).revoked))
	}
}
