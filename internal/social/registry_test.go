package social

import (
	"testing"

	"github.com/simplyblock/postbrain/internal/config"
)

func TestNewRegistry_OnlyEnabledProviders_Included(t *testing.T) {
	cfg := config.OAuthConfig{
		BaseURL: "https://postbrain.example.com",
		Providers: map[string]config.ProviderConfig{
			"github": {Enabled: true, ClientID: "gh", ClientSecret: "sec"},
			"google": {Enabled: true, ClientID: "g", ClientSecret: "sec"},
			"gitlab": {Enabled: true, ClientID: "gl", ClientSecret: "sec", InstanceURL: "https://gitlab.com"},
		},
	}

	reg := NewRegistry(cfg)
	if len(reg) != 3 {
		t.Fatalf("registry size = %d, want 3", len(reg))
	}
	if _, ok := reg["github"]; !ok {
		t.Fatal("github provider missing")
	}
	if _, ok := reg["google"]; !ok {
		t.Fatal("google provider missing")
	}
	if _, ok := reg["gitlab"]; !ok {
		t.Fatal("gitlab provider missing")
	}
}

func TestNewRegistry_DisabledProvider_Excluded(t *testing.T) {
	cfg := config.OAuthConfig{
		BaseURL: "https://postbrain.example.com",
		Providers: map[string]config.ProviderConfig{
			"github": {Enabled: true, ClientID: "gh", ClientSecret: "sec"},
			"google": {Enabled: false, ClientID: "g", ClientSecret: "sec"},
		},
	}

	reg := NewRegistry(cfg)
	if _, ok := reg["github"]; !ok {
		t.Fatal("github provider missing")
	}
	if _, ok := reg["google"]; ok {
		t.Fatal("google provider should be excluded when disabled")
	}
}

func TestNewRegistry_UnknownProvider_Ignored(t *testing.T) {
	cfg := config.OAuthConfig{
		BaseURL: "https://postbrain.example.com",
		Providers: map[string]config.ProviderConfig{
			"unknown_provider": {Enabled: true, ClientID: "x", ClientSecret: "y"},
		},
	}

	reg := NewRegistry(cfg)
	if len(reg) != 0 {
		t.Fatalf("registry size = %d, want 0", len(reg))
	}
}
