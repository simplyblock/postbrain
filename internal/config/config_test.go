package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/simplyblock/postbrain/internal/config"
)

func writeYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeYAML: %v", err)
	}
	return path
}

const fullConfig = `
database:
  url:             "postgres://postbrain:postbrain@localhost:5432/postbrain"
  auto_migrate:    true
  max_open:        25
  max_idle:        5
  connect_timeout: 10s

embedding:
  backend:         ollama
  service_url:     "http://localhost:11434"
  text_model:      "nomic-embed-text"
  code_model:      "nomic-embed-code"
  openai_api_key:  "sk-test"
  request_timeout: 30s
  batch_size:      64

server:
  addr:     ":7433"
  tls_cert: "/etc/tls/cert.pem"
  tls_key:  "/etc/tls/key.pem"

migrations:
  expected_version: 5

jobs:
  consolidation_enabled: true
  contradiction_enabled: false
  reembed_enabled:       true
  age_check_enabled:     false
`

// TestLoad_AllFields verifies that all config fields are correctly populated
// from a full YAML file.
func TestLoad_AllFields(t *testing.T) {
	path := writeYAML(t, fullConfig)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Database
	if cfg.Database.URL != "postgres://postbrain:postbrain@localhost:5432/postbrain" {
		t.Errorf("Database.URL = %q", cfg.Database.URL)
	}
	if !cfg.Database.AutoMigrate {
		t.Error("Database.AutoMigrate should be true")
	}
	if cfg.Database.MaxOpen != 25 {
		t.Errorf("Database.MaxOpen = %d", cfg.Database.MaxOpen)
	}
	if cfg.Database.MaxIdle != 5 {
		t.Errorf("Database.MaxIdle = %d", cfg.Database.MaxIdle)
	}
	if cfg.Database.ConnectTimeout != 10*time.Second {
		t.Errorf("Database.ConnectTimeout = %v", cfg.Database.ConnectTimeout)
	}

	// Embedding
	if cfg.Embedding.Backend != "ollama" {
		t.Errorf("Embedding.Backend = %q", cfg.Embedding.Backend)
	}
	if cfg.Embedding.ServiceURL != "http://localhost:11434" {
		t.Errorf("Embedding.ServiceURL = %q", cfg.Embedding.ServiceURL)
	}
	if cfg.Embedding.TextModel != "nomic-embed-text" {
		t.Errorf("Embedding.TextModel = %q", cfg.Embedding.TextModel)
	}
	if cfg.Embedding.CodeModel != "nomic-embed-code" {
		t.Errorf("Embedding.CodeModel = %q", cfg.Embedding.CodeModel)
	}
	if cfg.Embedding.OpenAIAPIKey != "sk-test" {
		t.Errorf("Embedding.OpenAIAPIKey = %q", cfg.Embedding.OpenAIAPIKey)
	}
	if cfg.Embedding.RequestTimeout != 30*time.Second {
		t.Errorf("Embedding.RequestTimeout = %v", cfg.Embedding.RequestTimeout)
	}
	if cfg.Embedding.BatchSize != 64 {
		t.Errorf("Embedding.BatchSize = %d", cfg.Embedding.BatchSize)
	}

	// Server
	if cfg.Server.Addr != ":7433" {
		t.Errorf("Server.Addr = %q", cfg.Server.Addr)
	}
	if cfg.Server.TLSCert != "/etc/tls/cert.pem" {
		t.Errorf("Server.TLSCert = %q", cfg.Server.TLSCert)
	}
	if cfg.Server.TLSKey != "/etc/tls/key.pem" {
		t.Errorf("Server.TLSKey = %q", cfg.Server.TLSKey)
	}

	// Migrations
	if cfg.Migrations.ExpectedVersion != 5 {
		t.Errorf("Migrations.ExpectedVersion = %d", cfg.Migrations.ExpectedVersion)
	}

	// Jobs
	if !cfg.Jobs.ConsolidationEnabled {
		t.Error("Jobs.ConsolidationEnabled should be true")
	}
	if cfg.Jobs.ContradictionEnabled {
		t.Error("Jobs.ContradictionEnabled should be false")
	}
	if !cfg.Jobs.ReembedEnabled {
		t.Error("Jobs.ReembedEnabled should be true")
	}
	if cfg.Jobs.AgeCheckEnabled {
		t.Error("Jobs.AgeCheckEnabled should be false")
	}
}

// TestLoad_MissingDatabaseURL verifies that a missing database.url returns an error.
func TestLoad_MissingDatabaseURL(t *testing.T) {
	path := writeYAML(t, `
server:
  addr: ":7433"
`)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for missing database.url, got nil")
	}
}

// TestLoad_EnvOverride verifies that POSTBRAIN_DATABASE_URL overrides the YAML value.
func TestLoad_EnvOverride(t *testing.T) {
	path := writeYAML(t, `
database:
  url: "postgres://localhost/postbrain"
`)
	t.Setenv("POSTBRAIN_DATABASE_URL", "postgres://override:override@remotehost/postbrain")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Database.URL != "postgres://override:override@remotehost/postbrain" {
		t.Errorf("Database.URL = %q, want override value", cfg.Database.URL)
	}
}

// TestLoad_Defaults verifies that omitted keys receive the expected default values.
func TestLoad_Defaults(t *testing.T) {
	path := writeYAML(t, `
database:
  url: "postgres://localhost/postbrain"
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Database.MaxOpen != 25 {
		t.Errorf("default Database.MaxOpen = %d, want 25", cfg.Database.MaxOpen)
	}
	if cfg.Database.MaxIdle != 5 {
		t.Errorf("default Database.MaxIdle = %d, want 5", cfg.Database.MaxIdle)
	}
	if cfg.Database.ConnectTimeout != 10*time.Second {
		t.Errorf("default Database.ConnectTimeout = %v, want 10s", cfg.Database.ConnectTimeout)
	}
	if cfg.Embedding.Backend != "ollama" {
		t.Errorf("default Embedding.Backend = %q, want ollama", cfg.Embedding.Backend)
	}
	if cfg.Embedding.ServiceURL != "" {
		t.Errorf("default Embedding.ServiceURL = %q, want empty", cfg.Embedding.ServiceURL)
	}
	if cfg.Embedding.BatchSize != 64 {
		t.Errorf("default Embedding.BatchSize = %d, want 64", cfg.Embedding.BatchSize)
	}
	if cfg.Server.Addr != ":7433" {
		t.Errorf("default Server.Addr = %q, want :7433", cfg.Server.Addr)
	}
	if cfg.OAuth.Server.AuthCodeTTL != 10*time.Minute {
		t.Errorf("default OAuth.Server.AuthCodeTTL = %v, want 10m", cfg.OAuth.Server.AuthCodeTTL)
	}
	if cfg.OAuth.Server.StateTTL != 15*time.Minute {
		t.Errorf("default OAuth.Server.StateTTL = %v, want 15m", cfg.OAuth.Server.StateTTL)
	}
	if cfg.OAuth.Server.TokenTTL != 0 {
		t.Errorf("default OAuth.Server.TokenTTL = %v, want 0", cfg.OAuth.Server.TokenTTL)
	}
	if !cfg.OAuth.Server.DynamicRegistration {
		t.Error("default OAuth.Server.DynamicRegistration should be true")
	}
	if !cfg.OAuth.Social.AutoCreateUsers {
		t.Error("default OAuth.Social.AutoCreateUsers should be true")
	}
	if cfg.OAuth.Social.RequireVerifiedEmail {
		t.Error("default OAuth.Social.RequireVerifiedEmail should be false")
	}
	if len(cfg.OAuth.Social.AllowedEmailDomains) != 0 {
		t.Errorf("default OAuth.Social.AllowedEmailDomains = %v, want empty", cfg.OAuth.Social.AllowedEmailDomains)
	}
}

func TestLoad_OAuthProviderRoundTrip(t *testing.T) {
	path := writeYAML(t, `
database:
  url: "postgres://localhost/postbrain"
oauth:
  base_url: "https://postbrain.example.com"
  social:
    auto_create_users: false
    require_verified_email: true
    allowed_email_domains: ["example.com"]
  providers:
    github:
      enabled: true
      client_id: "gh-client-id"
      client_secret: "gh-client-secret"
      scopes: ["read:user", "user:email"]
`)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.OAuth.BaseURL != "https://postbrain.example.com" {
		t.Errorf("OAuth.BaseURL = %q", cfg.OAuth.BaseURL)
	}
	gh, ok := cfg.OAuth.Providers["github"]
	if !ok {
		t.Fatal("OAuth.Providers[github] missing")
	}
	if !gh.Enabled {
		t.Error("OAuth.Providers[github].Enabled should be true")
	}
	if gh.ClientID != "gh-client-id" {
		t.Errorf("OAuth.Providers[github].ClientID = %q", gh.ClientID)
	}
	if gh.ClientSecret != "gh-client-secret" {
		t.Errorf("OAuth.Providers[github].ClientSecret = %q", gh.ClientSecret)
	}
	if len(gh.Scopes) != 2 || gh.Scopes[0] != "read:user" || gh.Scopes[1] != "user:email" {
		t.Errorf("OAuth.Providers[github].Scopes = %v", gh.Scopes)
	}
	if cfg.OAuth.Social.AutoCreateUsers {
		t.Error("OAuth.Social.AutoCreateUsers should be false")
	}
	if !cfg.OAuth.Social.RequireVerifiedEmail {
		t.Error("OAuth.Social.RequireVerifiedEmail should be true")
	}
	if len(cfg.OAuth.Social.AllowedEmailDomains) != 1 || cfg.OAuth.Social.AllowedEmailDomains[0] != "example.com" {
		t.Errorf("OAuth.Social.AllowedEmailDomains = %v", cfg.OAuth.Social.AllowedEmailDomains)
	}
}

func TestLoad_OAuthServerDurationParsing(t *testing.T) {
	path := writeYAML(t, `
database:
  url: "postgres://localhost/postbrain"
oauth:
  server:
    auth_code_ttl: 7m
    state_ttl: 21m
    token_ttl: 30m
    dynamic_registration: false
`)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.OAuth.Server.AuthCodeTTL != 7*time.Minute {
		t.Errorf("OAuth.Server.AuthCodeTTL = %v, want 7m", cfg.OAuth.Server.AuthCodeTTL)
	}
	if cfg.OAuth.Server.StateTTL != 21*time.Minute {
		t.Errorf("OAuth.Server.StateTTL = %v, want 21m", cfg.OAuth.Server.StateTTL)
	}
	if cfg.OAuth.Server.TokenTTL != 30*time.Minute {
		t.Errorf("OAuth.Server.TokenTTL = %v, want 30m", cfg.OAuth.Server.TokenTTL)
	}
	if cfg.OAuth.Server.DynamicRegistration {
		t.Error("OAuth.Server.DynamicRegistration should be false")
	}
}

func TestLoad_LegacyEmbeddingURLKeys_BackCompat(t *testing.T) {
	path := writeYAML(t, `
database:
  url: "postgres://localhost/postbrain"
embedding:
  backend: openai
  openai_base_url: "http://localhost:8080/v1"
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Embedding.ServiceURL != "http://localhost:8080/v1" {
		t.Fatalf("Embedding.ServiceURL = %q", cfg.Embedding.ServiceURL)
	}
}

func TestLoad_EmbeddingProviders_DefaultProfileSynthesized(t *testing.T) {
	path := writeYAML(t, `
database:
  url: "postgres://localhost/postbrain"
embedding:
  backend: openai
  service_url: "http://localhost:8080/v1"
  openai_api_key: "sk-test"
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Embedding.Providers) == 0 {
		t.Fatal("expected default provider profile to be synthesized")
	}
	p, ok := cfg.Embedding.Providers["default"]
	if !ok {
		t.Fatal("expected providers.default")
	}
	if p.Backend != "openai" || p.ServiceURL != "http://localhost:8080/v1" || p.OpenAIAPIKey != "sk-test" {
		t.Fatalf("unexpected providers.default: %+v", p)
	}
}

func TestLoad_EmbeddingProviders_CustomProfiles(t *testing.T) {
	path := writeYAML(t, `
database:
  url: "postgres://localhost/postbrain"
embedding:
  backend: ollama
  providers:
    local:
      backend: ollama
      service_url: "http://localhost:11434"
    openai-prod:
      backend: openai
      service_url: "https://api.openai.com/v1"
      openai_api_key: "sk-prod"
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Embedding.Providers) != 2 {
		t.Fatalf("providers len = %d, want 2", len(cfg.Embedding.Providers))
	}
	if cfg.Embedding.Providers["local"].Backend != "ollama" {
		t.Fatalf("providers.local.backend = %q", cfg.Embedding.Providers["local"].Backend)
	}
	if cfg.Embedding.Providers["openai-prod"].OpenAIAPIKey != "sk-prod" {
		t.Fatalf("providers.openai-prod.openai_api_key = %q", cfg.Embedding.Providers["openai-prod"].OpenAIAPIKey)
	}
}
