// Package config provides viper-based configuration loading for Postbrain.
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds the complete Postbrain runtime configuration.
type Config struct {
	Database   DatabaseConfig   `mapstructure:"database"`
	Embedding  EmbeddingConfig  `mapstructure:"embedding"`
	Server     ServerConfig     `mapstructure:"server"`
	Migrations MigrationsConfig `mapstructure:"migrations"`
	Jobs       JobsConfig       `mapstructure:"jobs"`
	OAuth      OAuthConfig      `mapstructure:"oauth"`
}

// DatabaseConfig holds PostgreSQL connection parameters.
type DatabaseConfig struct {
	URL            string        `mapstructure:"url"`
	AutoMigrate    bool          `mapstructure:"auto_migrate"`
	MaxOpen        int           `mapstructure:"max_open"`
	MaxIdle        int           `mapstructure:"max_idle"`
	ConnectTimeout time.Duration `mapstructure:"connect_timeout"`
}

// EmbeddingConfig holds embedding service parameters.
type EmbeddingConfig struct {
	Backend        string        `mapstructure:"backend"`
	OllamaURL      string        `mapstructure:"ollama_url"`
	TextModel      string        `mapstructure:"text_model"`
	CodeModel      string        `mapstructure:"code_model"`
	SummaryModel   string        `mapstructure:"summary_model"`
	OpenAIAPIKey   string        `mapstructure:"openai_api_key"`
	RequestTimeout time.Duration `mapstructure:"request_timeout"`
	BatchSize      int           `mapstructure:"batch_size"`
}

// ServerConfig holds HTTP/MCP server parameters.
type ServerConfig struct {
	Addr    string `mapstructure:"addr"`
	TLSCert string `mapstructure:"tls_cert"`
	TLSKey  string `mapstructure:"tls_key"`
}

// MigrationsConfig holds schema migration parameters.
type MigrationsConfig struct {
	ExpectedVersion int `mapstructure:"expected_version"`
}

// JobsConfig controls which background jobs are enabled at startup.
type JobsConfig struct {
	ConsolidationEnabled     bool `mapstructure:"consolidation_enabled"`
	ContradictionEnabled     bool `mapstructure:"contradiction_enabled"`
	ReembedEnabled           bool `mapstructure:"reembed_enabled"`
	AgeCheckEnabled          bool `mapstructure:"age_check_enabled"`
	BackfillSummariesEnabled bool `mapstructure:"backfill_summaries_enabled"`
	ChunkBackfillEnabled     bool `mapstructure:"chunk_backfill_enabled"`
}

// OAuthConfig holds social login and OAuth server settings.
type OAuthConfig struct {
	BaseURL   string                    `mapstructure:"base_url"`
	Providers map[string]ProviderConfig `mapstructure:"providers"`
	Server    OAuthServerConfig         `mapstructure:"server"`
}

// ProviderConfig holds social provider OAuth client settings.
type ProviderConfig struct {
	Enabled      bool     `mapstructure:"enabled"`
	ClientID     string   `mapstructure:"client_id"`
	ClientSecret string   `mapstructure:"client_secret"`
	Scopes       []string `mapstructure:"scopes"`
	InstanceURL  string   `mapstructure:"instance_url"`
}

// OAuthServerConfig holds authorization server runtime settings.
type OAuthServerConfig struct {
	AuthCodeTTL         time.Duration `mapstructure:"auth_code_ttl"`
	StateTTL            time.Duration `mapstructure:"state_ttl"`
	TokenTTL            time.Duration `mapstructure:"token_ttl"`
	DynamicRegistration bool          `mapstructure:"dynamic_registration"`
}

// LoadDatabaseURL reads only the database URL from the config file at path
// (or the POSTBRAIN_DATABASE_URL env var) without validating any other fields.
// It is intended for bootstrap commands like `onboard` that run before the
// full server config has been completed.
func LoadDatabaseURL(path string) (string, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("POSTBRAIN")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		// Config file may not exist yet — fall through to env var.
		if !errors.Is(err, viper.ConfigFileNotFoundError{}) {
			// Only return real read errors; missing file is OK if env var is set.
			_ = err
		}
	}

	url := v.GetString("database.url")
	if url == "" {
		return "", errors.New("database.url is required (set in config file or POSTBRAIN_DATABASE_URL env var)")
	}
	return url, nil
}

// Load reads configuration from the YAML file at path, applies defaults,
// overlays environment variables (prefix POSTBRAIN_), and validates required
// fields. It returns an error if database.url is empty.
func Load(path string) (*Config, error) {
	v := viper.New()

	// File source
	v.SetConfigFile(path)

	// Environment overrides: POSTBRAIN_DATABASE_URL → database.url
	v.SetEnvPrefix("POSTBRAIN")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Defaults matching config.example.yaml
	v.SetDefault("database.auto_migrate", true)
	v.SetDefault("database.max_open", 25)
	v.SetDefault("database.max_idle", 5)
	v.SetDefault("database.connect_timeout", "10s")

	v.SetDefault("embedding.backend", "ollama")
	v.SetDefault("embedding.ollama_url", "http://localhost:11434")
	v.SetDefault("embedding.text_model", "nomic-embed-text")
	v.SetDefault("embedding.code_model", "nomic-embed-code")
	v.SetDefault("embedding.openai_api_key", "")
	v.SetDefault("embedding.request_timeout", "30s")
	v.SetDefault("embedding.batch_size", 64)

	v.SetDefault("server.addr", ":7433")
	v.SetDefault("server.tls_cert", "")
	v.SetDefault("server.tls_key", "")

	v.SetDefault("migrations.expected_version", 0)

	v.SetDefault("jobs.consolidation_enabled", true)
	v.SetDefault("jobs.contradiction_enabled", true)
	v.SetDefault("jobs.reembed_enabled", true)
	v.SetDefault("jobs.age_check_enabled", true)

	v.SetDefault("oauth.server.auth_code_ttl", "10m")
	v.SetDefault("oauth.server.state_ttl", "15m")
	v.SetDefault("oauth.server.token_ttl", "0")
	v.SetDefault("oauth.server.dynamic_registration", true)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("config: read %q: %w", path, err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config: unmarshal: %w", err)
	}

	// Validate required fields
	if cfg.Database.URL == "" {
		return nil, errors.New("database.url is required")
	}

	return &cfg, nil
}
