package social

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/simplyblock/postbrain/internal/config"
)

func TestGitHubProvider_AuthURL_ContainsClientIDAndState(t *testing.T) {
	p := NewGitHubProvider("https://postbrain.example.com", config.ProviderConfig{
		ClientID:     "gh-client-id",
		ClientSecret: "gh-secret",
		Scopes:       []string{"read:user", "user:email"},
	})
	u := p.AuthURL("state-123")
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	q := parsed.Query()
	if q.Get("client_id") != "gh-client-id" {
		t.Fatalf("client_id = %q", q.Get("client_id"))
	}
	if q.Get("state") != "state-123" {
		t.Fatalf("state = %q", q.Get("state"))
	}
	if q.Get("redirect_uri") != "https://postbrain.example.com/ui/auth/github/callback" {
		t.Fatalf("redirect_uri = %q", q.Get("redirect_uri"))
	}
}

func TestGitHubProvider_Exchange_ValidCode_ReturnsUserInfo(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "gh-token"})
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         42,
			"login":      "octo",
			"name":       "Octo Cat",
			"avatar_url": "https://cdn.example/avatar.png",
			"email":      "fallback@example.com",
		})
	})
	mux.HandleFunc("/user/emails", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"email": "primary@example.com", "primary": true, "verified": true}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := NewGitHubProvider("https://postbrain.example.com", config.ProviderConfig{
		ClientID:     "gh-client-id",
		ClientSecret: "gh-secret",
		Scopes:       []string{"read:user", "user:email"},
	})
	p.authorizeURL = srv.URL + "/authorize"
	p.tokenURL = srv.URL + "/login/oauth/access_token"
	p.userURL = srv.URL + "/user"
	p.emailsURL = srv.URL + "/user/emails"
	p.httpClient = srv.Client()

	info, err := p.Exchange(context.Background(), "valid-code")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if info.ProviderID != "42" {
		t.Fatalf("provider id = %q", info.ProviderID)
	}
	if info.Email != "primary@example.com" {
		t.Fatalf("email = %q", info.Email)
	}
	if info.DisplayName != "Octo Cat" {
		t.Fatalf("display name = %q", info.DisplayName)
	}
}

func TestGitHubProvider_Exchange_APIError_ReturnsError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad", http.StatusBadGateway)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := NewGitHubProvider("https://postbrain.example.com", config.ProviderConfig{
		ClientID:     "gh-client-id",
		ClientSecret: "gh-secret",
		Scopes:       []string{"read:user", "user:email"},
	})
	p.tokenURL = srv.URL + "/login/oauth/access_token"
	p.httpClient = srv.Client()

	if _, err := p.Exchange(context.Background(), "invalid"); err == nil {
		t.Fatal("Exchange expected error, got nil")
	}
}

func TestGitHubProvider_Exchange_UsesVerifiedPrimaryEmail(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "gh-token"})
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         43,
			"login":      "octo2",
			"name":       "Octo Cat 2",
			"avatar_url": "https://cdn.example/avatar2.png",
			"email":      "fallback@example.com",
		})
	})
	mux.HandleFunc("/user/emails", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"email": "not-verified@example.com", "primary": true, "verified": false},
			{"email": "verified-primary@example.com", "primary": true, "verified": true},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := NewGitHubProvider("https://postbrain.example.com", config.ProviderConfig{
		ClientID:     "gh-client-id",
		ClientSecret: "gh-secret",
		Scopes:       []string{"read:user", "user:email"},
	})
	p.tokenURL = srv.URL + "/login/oauth/access_token"
	p.userURL = srv.URL + "/user"
	p.emailsURL = srv.URL + "/user/emails"
	p.httpClient = srv.Client()

	info, err := p.Exchange(context.Background(), "valid-code")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if !strings.EqualFold(info.Email, "verified-primary@example.com") {
		t.Fatalf("email = %q, want verified primary", info.Email)
	}
}
