package social

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/simplyblock/postbrain/internal/config"
)

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
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"token_endpoint": "http://example.test/unused"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux = http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"token_endpoint": srv.URL + "/token"})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id_token": fakeJWT(map[string]any{
			"sub":     "google-sub-1",
			"email":   "user@example.com",
			"name":    "Google User",
			"picture": "https://cdn.example/google.png",
		})})
	})
	srv.Config.Handler = mux

	p := NewGoogleProvider("https://postbrain.example.com", config.ProviderConfig{
		ClientID:     "google-client-id",
		ClientSecret: "google-secret",
		Scopes:       []string{"openid", "email", "profile"},
	})
	p.discoveryURL = srv.URL + "/.well-known/openid-configuration"
	p.httpClient = srv.Client()

	info, err := p.Exchange(context.Background(), "valid-code")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if info.ProviderID != "google-sub-1" {
		t.Fatalf("provider id = %q", info.ProviderID)
	}
	if info.Email != "user@example.com" {
		t.Fatalf("email = %q", info.Email)
	}
}

func TestGoogleProvider_Exchange_InvalidIDToken_ReturnsError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"token_endpoint": "http://example.test/token"})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id_token": "not-a-jwt"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := NewGoogleProvider("https://postbrain.example.com", config.ProviderConfig{
		ClientID:     "google-client-id",
		ClientSecret: "google-secret",
		Scopes:       []string{"openid", "email", "profile"},
	})
	p.discoveryURL = srv.URL + "/.well-known/openid-configuration"
	p.httpClient = srv.Client()

	if _, err := p.Exchange(context.Background(), "bad-code"); err == nil {
		t.Fatal("Exchange expected error, got nil")
	}
}

func fakeJWT(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payloadBytes, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	return strings.Join([]string{header, payload, "sig"}, ".")
}
