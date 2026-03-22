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
  ollama_url:      "http://localhost:11434"
  text_model:      "nomic-embed-text"
  code_model:      "nomic-embed-code"
  openai_api_key:  "sk-test"
  request_timeout: 30s
  batch_size:      64

server:
  addr:     ":7433"
  token:    "supersecret"
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
	if cfg.Embedding.OllamaURL != "http://localhost:11434" {
		t.Errorf("Embedding.OllamaURL = %q", cfg.Embedding.OllamaURL)
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
	if cfg.Server.Token != "supersecret" {
		t.Errorf("Server.Token = %q", cfg.Server.Token)
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
  token: "supersecret"
`)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for missing database.url, got nil")
	}
}

// TestLoad_MissingServerToken verifies that a missing server.token returns an error.
func TestLoad_MissingServerToken(t *testing.T) {
	path := writeYAML(t, `
database:
  url: "postgres://localhost/postbrain"
`)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for missing server.token, got nil")
	}
}

// TestLoad_ChangemeToken verifies that token "changeme" is accepted (just warns).
func TestLoad_ChangemeToken(t *testing.T) {
	path := writeYAML(t, `
database:
  url: "postgres://localhost/postbrain"
server:
  token: "changeme"
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("expected no error for changeme token, got: %v", err)
	}
	if cfg.Server.Token != "changeme" {
		t.Errorf("Server.Token = %q", cfg.Server.Token)
	}
}

// TestLoad_EnvOverride verifies that POSTBRAIN_DATABASE_URL overrides the YAML value.
func TestLoad_EnvOverride(t *testing.T) {
	path := writeYAML(t, `
database:
  url: "postgres://localhost/postbrain"
server:
  token: "supersecret"
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
server:
  token: "supersecret"
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
	if cfg.Embedding.BatchSize != 64 {
		t.Errorf("default Embedding.BatchSize = %d, want 64", cfg.Embedding.BatchSize)
	}
	if cfg.Server.Addr != ":7433" {
		t.Errorf("default Server.Addr = %q, want :7433", cfg.Server.Addr)
	}
}
