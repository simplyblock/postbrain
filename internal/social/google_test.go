package social

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	josejwt "github.com/go-jose/go-jose/v4/jwt"

	"github.com/simplyblock/postbrain/internal/config"
)

// oidcTestServer is a minimal OIDC provider that signs tokens with an RSA key.
// It satisfies the go-oidc library's discovery + JWKS requirements.
type oidcTestServer struct {
	srv      *httptest.Server
	key      *rsa.PrivateKey
	clientID string
}

func newOIDCTestServer(t *testing.T, clientID string) *oidcTestServer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	ts := &oidcTestServer{key: key, clientID: clientID}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", ts.handleDiscovery)
	mux.HandleFunc("/jwks", ts.handleJWKS)
	ts.srv = httptest.NewServer(mux)
	t.Cleanup(ts.srv.Close)
	return ts
}

func (ts *oidcTestServer) handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"issuer":                                ts.srv.URL,
		"authorization_endpoint":                ts.srv.URL + "/auth",
		"token_endpoint":                        ts.srv.URL + "/token",
		"jwks_uri":                              ts.srv.URL + "/jwks",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
	})
}

func (ts *oidcTestServer) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	jwks := jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{
			{Key: &ts.key.PublicKey, KeyID: "test-key-1", Algorithm: string(jose.RS256), Use: "sig"},
		},
	}
	_ = json.NewEncoder(w).Encode(jwks)
}

// signToken creates a valid RS256-signed JWT with the given extra claims.
func (ts *oidcTestServer) signToken(t *testing.T, extra map[string]any) string {
	t.Helper()
	sig, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: ts.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "test-key-1"),
	)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	stdClaims := josejwt.Claims{
		Issuer:   ts.srv.URL,
		Subject:  "google-sub-1",
		Audience: josejwt.Audience{ts.clientID},
		Expiry:   josejwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt: josejwt.NewNumericDate(time.Now()),
	}
	raw, err := josejwt.Signed(sig).Claims(stdClaims).Claims(extra).Serialize()
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return raw
}

// forgeAlgNoneJWT builds a JWT with alg:none — no cryptographic signature.
// This is the token format from the SAST proof-of-concept.
func forgeAlgNoneJWT(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, _ := json.Marshal(claims)
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
}

// newTestProvider wires a GoogleProvider against the test OIDC server.
func newTestProvider(ts *oidcTestServer) *GoogleProvider {
	return &GoogleProvider{
		clientID:     ts.clientID,
		clientSecret: "secret",
		redirectURI:  "https://postbrain.example.com/ui/auth/google/callback",
		httpClient:   ts.srv.Client(),
		issuerURL:    ts.srv.URL,
	}
}

func TestGoogleProvider_AuthURL_ContainsClientIDAndState(t *testing.T) {
	p := NewGoogleProvider("https://postbrain.example.com", config.ProviderConfig{
		ClientID:     "google-client-id",
		ClientSecret: "google-secret",
		Scopes:       []string{"openid", "email", "profile"},
	})
	u := p.AuthURL("state-123")
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	q := parsed.Query()
	if q.Get("client_id") != "google-client-id" {
		t.Fatalf("client_id = %q", q.Get("client_id"))
	}
	if q.Get("state") != "state-123" {
		t.Fatalf("state = %q", q.Get("state"))
	}
	if q.Get("redirect_uri") != "https://postbrain.example.com/ui/auth/google/callback" {
		t.Fatalf("redirect_uri = %q", q.Get("redirect_uri"))
	}
}

func TestGoogleProvider_Exchange_ValidIDToken_ReturnsUserInfo(t *testing.T) {
	ts := newOIDCTestServer(t, "google-client-id")
	signedToken := ts.signToken(t, map[string]any{
		"email":          "user@example.com",
		"email_verified": true,
		"name":           "Google User",
		"picture":        "https://cdn.example/google.png",
	})
	ts.srv.Config.Handler.(*http.ServeMux).HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id_token": signedToken})
	})

	info, err := newTestProvider(ts).Exchange(context.Background(), "valid-code")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if info.ProviderID != "google-sub-1" {
		t.Fatalf("provider id = %q, want google-sub-1", info.ProviderID)
	}
	if info.Email != "user@example.com" {
		t.Fatalf("email = %q, want user@example.com", info.Email)
	}
	if info.DisplayName != "Google User" {
		t.Fatalf("display name = %q, want Google User", info.DisplayName)
	}
}

// TestGoogleProvider_Exchange_ForgedAlgNone_Rejected verifies that a JWT with alg:none
// (no signature) is rejected — this is the exact attack described in the SAST report.
func TestGoogleProvider_Exchange_ForgedAlgNone_Rejected(t *testing.T) {
	ts := newOIDCTestServer(t, "google-client-id")
	forgedToken := forgeAlgNoneJWT(map[string]any{
		"iss":            ts.srv.URL,
		"sub":            "victim-sub",
		"aud":            "google-client-id",
		"email":          "victim@example.com",
		"email_verified": true,
		"exp":            time.Now().Add(time.Hour).Unix(),
		"iat":            time.Now().Unix(),
	})
	ts.srv.Config.Handler.(*http.ServeMux).HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id_token": forgedToken})
	})

	_, err := newTestProvider(ts).Exchange(context.Background(), "forged-code")
	if err == nil {
		t.Fatal("Exchange: expected error for alg:none forged token, got nil — authentication bypass possible")
	}
}

func TestGoogleProvider_Exchange_MalformedIDToken_ReturnsError(t *testing.T) {
	ts := newOIDCTestServer(t, "google-client-id")
	ts.srv.Config.Handler.(*http.ServeMux).HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id_token": "not-a-jwt"})
	})

	if _, err := newTestProvider(ts).Exchange(context.Background(), "bad-code"); err == nil {
		t.Fatal("Exchange: expected error for malformed token, got nil")
	}
}