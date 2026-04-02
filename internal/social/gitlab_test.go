package social

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/simplyblock/postbrain/internal/config"
)

func TestGitLabProvider_AuthURL_UsesInstanceURL(t *testing.T) {
	p := NewGitLabProvider("https://postbrain.example.com", config.ProviderConfig{
		ClientID:     "gitlab-client-id",
		ClientSecret: "gitlab-secret",
		Scopes:       []string{"read_user"},
		InstanceURL:  "https://gitlab.example.com",
	})
	u := p.AuthURL("state-xyz")
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if parsed.Host != "gitlab.example.com" {
		t.Fatalf("host = %q, want gitlab.example.com", parsed.Host)
	}
	if parsed.Query().Get("state") != "state-xyz" {
		t.Fatalf("state = %q", parsed.Query().Get("state"))
	}
}

func TestGitLabProvider_Exchange_ValidCode_ReturnsUserInfo(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "gl-token"})
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         77,
			"email":      "gitlab@example.com",
			"name":       "GitLab User",
			"avatar_url": "https://cdn.example/gl.png",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := NewGitLabProvider("https://postbrain.example.com", config.ProviderConfig{
		ClientID:     "gitlab-client-id",
		ClientSecret: "gitlab-secret",
		Scopes:       []string{"read_user"},
		InstanceURL:  srv.URL,
	})
	p.httpClient = srv.Client()

	info, err := p.Exchange(context.Background(), "valid-code")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if info.ProviderID != "77" {
		t.Fatalf("provider id = %q", info.ProviderID)
	}
	if info.Email != "gitlab@example.com" {
		t.Fatalf("email = %q", info.Email)
	}
}
